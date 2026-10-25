package com.appcontrol.child.policy;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.Collections;
import java.util.List;

public final class ScheduledPolicyChecks {
    private static List<String> names(String csv) { return csv.equals("-") ? Collections.emptyList() : Arrays.asList(csv.split(",")); }
    private static ScheduledPolicy policy(ScheduledPolicy.Rule rule) { return new ScheduledPolicy(7, false, Collections.emptyList(), Collections.emptyList(), Collections.singleton(rule)); }
    private static void require(boolean value, String message) { if (!value) throw new AssertionError(message); }
    private static void boundary(ScheduledPolicy policy, String now, String expected) {
        require(Instant.parse(expected).equals(policy.resolve(Instant.parse(now)).nextBoundary), "incorrect boundary at " + now);
    }
    private static void invalid(Runnable action) {
        try { action.run(); throw new AssertionError("invalid policy accepted"); } catch (RuntimeException expected) { }
    }
    public static void main(String[] args) throws Exception {
        int count = 0;
        for (String line : Files.readAllLines(Paths.get(args[0]), StandardCharsets.UTF_8)) {
            if (line.isBlank() || line.startsWith("#")) continue;
            String[] f = line.split("\t", -1);
            require(f.length == 14, "invalid vector width");
            List<Integer> days = new ArrayList<>(); for (String day : f[3].split(",")) days.add(Integer.parseInt(day));
            ScheduledPolicy.Rule rule = new ScheduledPolicy.Rule("window", f[2], days, Integer.parseInt(f[4]), Integer.parseInt(f[5]), Boolean.parseBoolean(f[7]), names(f[9]));
            ScheduledPolicy.Effective result = new ScheduledPolicy(7, Boolean.parseBoolean(f[6]), names(f[8]), names(f[10]), Collections.singleton(rule)).resolve(Instant.parse(f[1]));
            require(result.internetPaused == Boolean.parseBoolean(f[11]), "pause mismatch: " + f[0]);
            require(result.blockedPackages.equals(names(f[12])), "blocked packages mismatch: " + f[0]);
            require(result.stateKey.equals(f[13]), "server/child state hash mismatch: " + f[0]);
            count++;
        }
        require(count == 12, "missing shared vectors");
        ScheduledPolicy night = policy(new ScheduledPolicy.Rule("night", "Asia/Kolkata", List.of(5), 1320, 420, true, List.of("org.example.game")));
        boundary(night, "2026-10-23T16:29:59Z", "2026-10-23T16:30:00Z");
        boundary(night, "2026-10-23T16:30:00Z", "2026-10-24T01:30:00Z");
        ScheduledPolicy spring = policy(new ScheduledPolicy.Rule("gap", "America/New_York", List.of(7), 150, 240, true, Collections.emptyList()));
        boundary(spring, "2026-03-08T06:59:00Z", "2026-03-08T07:00:00Z");
        ScheduledPolicy fall = policy(new ScheduledPolicy.Rule("repeat", "America/New_York", List.of(7), 75, 105, true, Collections.emptyList()));
        boundary(fall, "2026-11-01T05:30:00Z", "2026-11-01T05:45:00Z");
        boundary(fall, "2026-11-01T05:50:00Z", "2026-11-01T06:00:00Z");
        boundary(fall, "2026-11-01T06:00:00Z", "2026-11-01T06:15:00Z");
        boundary(fall, "2026-11-01T06:30:00Z", "2026-11-01T06:45:00Z");
        ScheduledPolicy.Rule first = new ScheduledPolicy.Rule("school", "UTC", List.of(5), 600, 900, true, List.of("org.example.game", "org.example.safe"));
        ScheduledPolicy.Rule second = new ScheduledPolicy.Rule("study", "UTC", List.of(5), 660, 840, false, List.of("org.example.study"));
        ScheduledPolicy.Effective overlap = new ScheduledPolicy(7, false, List.of("org.example.base"), List.of("org.example.safe"), List.of(first, second)).resolve(Instant.parse("2026-10-23T12:00:00Z"));
        require(overlap.internetPaused && overlap.blockedPackages.equals(List.of("org.example.base", "org.example.game", "org.example.study")), "overlap or allow override failed");
        require(overlap.stateKey.equals(new ScheduledPolicy(7, false, List.of("org.example.base"), List.of("org.example.safe"), List.of(second, first)).resolve(Instant.parse("2026-10-23T12:00:00Z")).stateKey), "rule order changed effective hash");
        invalid(() -> new ScheduledPolicy.Rule("bad", "UTC", List.of(1, 1), 0, 60, false, Collections.emptyList()));
        invalid(() -> new ScheduledPolicy.Rule("bad", "UTC", List.of(1), 60, 60, false, Collections.emptyList()));
        invalid(() -> new ScheduledPolicy.Rule("bad", "Not/AZone", List.of(1), 0, 60, false, Collections.emptyList()));
        invalid(() -> new ScheduledPolicy(7, false, List.of("com.android.phone"), Collections.emptyList(), Collections.emptyList()));
        invalid(() -> new ScheduledPolicy(7, false, Collections.emptyList(), Collections.emptyList(), List.of(first, first)));
        require(InternetPausePlan.targets("com.appcontrol.child", List.of(new InternetPausePlan.App("org.example.safe", false, false)), List.of("org.example.safe")).isEmpty(), "always-allowed app entered VPN");
        System.out.println("schedule-checks-passed: 12 shared vectors, overnight/weekly boundaries, DST gaps/repeated hours, overlaps, allow override, validation and VPN exemption");
    }
}
