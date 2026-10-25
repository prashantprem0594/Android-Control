package com.appcontrol.child.wakeup;

import java.util.Base64;
import java.util.HashMap;
import java.util.Map;

public final class WakeupHintChecks {
    private static void require(boolean value) { if (!value) throw new AssertionError("Wake-up data invariant failed"); }
    public static void main(String[] args) {
        byte[] nonce = new byte[16];
        for (int i = 0; i < 256; i++) {
            nonce[15] = (byte)i;
            String value = Base64.getUrlEncoder().withoutPadding().encodeToString(nonce);
            require(WakeupHint.accepts(Map.of("syncHint", value), false));
            require(!WakeupHint.accepts(Map.of("syncHint", value), true));
            require(!WakeupHint.accepts(Map.of("syncHint", value, "type", "LOCK_DEVICE"), false));
            require(!WakeupHint.accepts(Map.of("syncHint", value + "=="), false));
        }
        require(!WakeupHint.accepts(null, false));
        require(!WakeupHint.accepts(Map.of(), false));
        for (String rejected : new String[]{"", "command", "A".repeat(21), "A".repeat(23), "A".repeat(21)+"B", "A".repeat(21)+"/", "A".repeat(21)+"\n"})
            require(!WakeupHint.accepts(Map.of("syncHint", rejected), false));
        Map<String,String> missing = new HashMap<>(); missing.put("syncHint", null);
        require(!WakeupHint.accepts(missing, false));
        require(!WakeupHint.accepts(Map.of("command", "A".repeat(22)), false));
        System.out.println("Wake-up data checks passed: canonical opaque nonce only; notifications, commands, extra fields and malformed data rejected.");
    }
}
