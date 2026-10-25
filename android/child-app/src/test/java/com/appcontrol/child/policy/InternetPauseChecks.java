package com.appcontrol.child.policy;

import java.util.Arrays;
import java.util.Collections;
import java.util.Set;

public final class InternetPauseChecks {
    public static void main(String[] args) {
        Set<String> targets = InternetPausePlan.targets("com.appcontrol.child", Arrays.asList(
            new InternetPausePlan.App("com.appcontrol.child", false, false),
            new InternetPausePlan.App("com.google.android.gms", false, false),
            new InternetPausePlan.App("com.android.phone", false, false),
            new InternetPausePlan.App("com.xiaomi.ims", true, false),
            new InternetPausePlan.App("org.example.phone", false, true),
            new InternetPausePlan.App("org.example.vpn", false, true),
            new InternetPausePlan.App("org.example.game", false, false),
            new InternetPausePlan.App("org.example.browser", false, false)));
        if (!targets.equals(Set.of("org.example.game", "org.example.browser")) || !InternetPausePlan.needsTunnel(targets)) {
            throw new AssertionError("management, FCM, system, dialer or VPN traffic could enter discard tunnel");
        }
        targets = InternetPausePlan.targets("com.appcontrol.child", Collections.singletonList(
            new InternetPausePlan.App("com.appcontrol.child", false, false)));
        if (InternetPausePlan.needsTunnel(targets) || InternetPausePlan.needsTunnel(Collections.emptySet())) {
            throw new AssertionError("empty target list would capture all traffic");
        }
        System.out.println("internet-pause-checks-passed: explicit user targets, management/system/role exclusions, empty-target no-tunnel");
    }
}
