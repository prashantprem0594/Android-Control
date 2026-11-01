package com.appcontrol.parent.push;

import java.util.Base64;
import java.util.Map;

/** Local callback boundary checks; no Android runtime, network or Firebase project needed. */
public final class ParentPushHintChecks {
    private static void require(boolean condition) {
        if (!condition) throw new AssertionError("Parent hint boundary failed");
    }

    public static void main(String[] args) {
        byte[] value = new byte[16];
        for (int index = 0; index < value.length; index++) value[index] = (byte) index;
        String nonce = Base64.getUrlEncoder().withoutPadding().encodeToString(value);
        require(ParentPushHint.accepts(Map.of("syncHint", nonce), false));
        require(!ParentPushHint.accepts(Map.of("syncHint", nonce), true));
        require(!ParentPushHint.accepts(null, false));
        require(!ParentPushHint.accepts(Map.of(), false));
        require(!ParentPushHint.accepts(Map.of("syncHint", nonce, "callId", "forbidden"), false));
        require(!ParentPushHint.accepts(Map.of("callId", nonce), false));
        for (String rejected : new String[]{"", nonce + "==", nonce.substring(0, 21), nonce + "A",
                "AAAAAAAAAAAAAAAAAAAAAB", "AAAAAAAAAAAAAAAAAAAAA/", " "+nonce, nonce+"\n"}) {
            require(!ParentPushHint.accepts(Map.of("syncHint", rejected), false));
        }
        System.out.println("parent-push-hint-checks-passed: opaque canonical nonce only; notification, call/extra data and malformed hints refused");
    }
}
