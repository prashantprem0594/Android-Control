package com.appcontrol.child.policy;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Instant;
import java.time.LocalDate;
import java.time.LocalDateTime;
import java.time.ZoneId;
import java.time.ZoneOffset;
import java.time.ZonedDateTime;
import java.time.zone.ZoneOffsetTransition;
import java.util.ArrayList;
import java.util.Collection;
import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.TreeSet;
import java.util.regex.Pattern;

/** Offline wall-clock recurrence. Weekdays are ISO 1=Monday through 7=Sunday. */
public final class ScheduledPolicy {
    private static final Pattern PACKAGE = Pattern.compile("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)+");
    public static final class Rule {
        public final String id;
        public final ZoneId zone;
        public final Set<Integer> days;
        public final int startMinute;
        public final int endMinute;
        public final boolean internetPaused;
        public final List<String> blockedPackages;
        public Rule(String id, String timeZone, Collection<Integer> days, int start, int end, boolean paused, Collection<String> blocked) {
            if (id == null || !id.matches("[A-Za-z0-9_-]{1,64}") || timeZone == null || timeZone.length() > 128 ||
                days.isEmpty() || days.size() > 7 || start < 0 || start > 1439 || end < 0 || end > 1440 || start == end) {
                throw new IllegalArgumentException("Invalid schedule");
            }
            this.id = id; this.zone = ZoneId.of(timeZone);
            this.days = Collections.unmodifiableSet(new HashSet<>(days));
            if (this.days.size() != days.size()) throw new IllegalArgumentException("Duplicate schedule day");
            for (Integer day : days) if (day == null || day < 1 || day > 7) throw new IllegalArgumentException("Invalid weekday");
            this.startMinute = start; this.endMinute = end; this.internetPaused = paused;
            this.blockedPackages = packages(blocked, true);
        }
        private boolean active(Instant now) {
            ZonedDateTime local = now.atZone(zone);
            int day = local.getDayOfWeek().getValue();
            int minute = local.getHour() * 60 + local.getMinute();
            if (startMinute < endMinute) return days.contains(day) && minute >= startMinute && minute < endMinute;
            return minute >= startMinute && days.contains(day) || minute < endMinute && days.contains((day + 5) % 7 + 1);
        }
    }
    public static final class Effective {
        public final boolean internetPaused;
        public final List<String> blockedPackages;
        public final List<String> alwaysAllowedPackages;
        public final String stateKey;
        public final Instant nextBoundary;
        private Effective(long version, boolean paused, Collection<String> blocked, List<String> allowed, Instant next) {
            internetPaused = paused;
            blockedPackages = Collections.unmodifiableList(new ArrayList<>(blocked));
            alwaysAllowedPackages = allowed;
            nextBoundary = next;
            String canonical = "app-control-effective-policy-v1\n" + version + "\n" + paused + "\n" +
                String.join("\n", blockedPackages) + "\n--allowed--\n" + String.join("\n", allowed) + "\n";
            try {
                byte[] digest = MessageDigest.getInstance("SHA-256").digest(canonical.getBytes(StandardCharsets.UTF_8));
                StringBuilder hex = new StringBuilder(64);
                for (byte value : digest) hex.append(Character.forDigit((value >>> 4) & 15, 16)).append(Character.forDigit(value & 15, 16));
                stateKey = hex.toString();
            } catch (java.security.NoSuchAlgorithmException impossible) { throw new IllegalStateException(impossible); }
        }
    }
    private final long version;
    private final boolean basePaused;
    private final List<String> baseBlocked;
    private final List<String> allowed;
    private final List<Rule> rules;

    public ScheduledPolicy(long version, boolean paused, Collection<String> blocked, Collection<String> allowed, Collection<Rule> rules) {
        if (version < 0 || version > (1L << 53) || rules.size() > 20) throw new IllegalArgumentException("Invalid policy");
        this.version = version; basePaused = paused; baseBlocked = packages(blocked, true);
        this.allowed = packages(allowed, false); this.rules = Collections.unmodifiableList(new ArrayList<>(rules));
        Set<String> ids = new HashSet<>(); Set<String> targets = new HashSet<>(baseBlocked);
        for (Rule rule : rules) {
            if (!ids.add(rule.id)) throw new IllegalArgumentException("Duplicate schedule id");
            targets.addAll(rule.blockedPackages);
        }
        if (targets.size() > 100) throw new IllegalArgumentException("Too many policy targets");
    }

    public Effective resolve(Instant now) {
        Set<String> blocked = new TreeSet<>(baseBlocked);
        boolean paused = basePaused;
        Instant next = null;
        for (Rule rule : rules) {
            if (rule.active(now)) { paused |= rule.internetPaused; blocked.addAll(rule.blockedPackages); }
            LocalDate today = now.atZone(rule.zone).toLocalDate();
            for (int offset = -1; offset <= 8; offset++) {
                LocalDate startDate = today.plusDays(offset);
                if (!rule.days.contains(startDate.getDayOfWeek().getValue())) continue;
                LocalDateTime start = startDate.atStartOfDay().plusMinutes(rule.startMinute);
                LocalDateTime end = startDate.atStartOfDay().plusMinutes(rule.endMinute);
                if (rule.endMinute < rule.startMinute) end = end.plusDays(1);
                next = earlier(next, futureBoundary(start, rule.zone, now));
                next = earlier(next, futureBoundary(end, rule.zone, now));
            }
            // A clock rollback can change active state even before a repeated boundary.
            ZoneOffsetTransition transition = rule.zone.getRules().nextTransition(now);
            if (transition != null && transition.getInstant().isBefore(now.plusSeconds(9 * 86400L))) {
                next = earlier(next, transition.getInstant());
            }
        }
        blocked.removeAll(allowed);
        return new Effective(version, paused, blocked, allowed, next);
    }

    private static Instant futureBoundary(LocalDateTime wall, ZoneId zone, Instant now) {
        List<ZoneOffset> offsets = zone.getRules().getValidOffsets(wall);
        Instant candidate = null;
        if (offsets.isEmpty()) {
            Instant transition = zone.getRules().getTransition(wall).getInstant();
            if (transition.isAfter(now)) candidate = transition;
        } else {
            for (ZoneOffset offset : offsets) {
                Instant value = wall.toInstant(offset);
                if (value.isAfter(now)) candidate = earlier(candidate, value);
            }
        }
        return candidate;
    }
    private static Instant earlier(Instant left, Instant right) {
        return left == null ? right : right == null || left.isBefore(right) ? left : right;
    }
    private static List<String> packages(Collection<String> input, boolean blocking) {
        if (input.size() > 100) throw new IllegalArgumentException("Too many packages");
        Set<String> names = new TreeSet<>();
        for (String name : input) {
            if (name == null || name.length() > 255 || !(name.equals("android") && !blocking || PACKAGE.matcher(name).matches()) || !names.add(name)) {
                throw new IllegalArgumentException("Invalid package list");
            }
            if (blocking && (name.equals("android") || name.startsWith("com.android.") || name.startsWith("com.google.android.") || name.startsWith("com.appcontrol."))) {
                throw new IllegalArgumentException("Protected package");
            }
        }
        return Collections.unmodifiableList(new ArrayList<>(names));
    }
}
