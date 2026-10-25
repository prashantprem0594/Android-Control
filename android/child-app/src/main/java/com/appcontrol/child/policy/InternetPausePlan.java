package com.appcontrol.child.policy;

import java.util.Collection;
import java.util.Set;
import java.util.TreeSet;
import java.util.Collections;

/** Only explicit non-critical user apps may enter the discard tunnel. */
public final class InternetPausePlan {
    public static final class App {
        public final String name;
        public final boolean system;
        public final boolean protectedRole;
        public App(String name, boolean system, boolean protectedRole) {
            this.name = name; this.system = system; this.protectedRole = protectedRole;
        }
    }
    public static Set<String> targets(String managementPackage, Collection<App> installed) {
        return targets(managementPackage, installed, Collections.emptySet());
    }
    public static Set<String> targets(String managementPackage, Collection<App> installed, Collection<String> alwaysAllowed) {
        Set<String> result = new TreeSet<>();
        for (App app : installed) {
            if (alwaysAllowed.contains(app.name) || app.system || app.protectedRole || app.name.equals(managementPackage) || app.name.equals("android") ||
                app.name.startsWith("com.android.") || app.name.startsWith("com.google.android.") ||
                app.name.startsWith("com.appcontrol.")) continue;
            result.add(app.name);
        }
        return result;
    }
    // A VPN with no per-app allow entries would capture all applications.
    public static boolean needsTunnel(Set<String> targets) { return !targets.isEmpty(); }
}
