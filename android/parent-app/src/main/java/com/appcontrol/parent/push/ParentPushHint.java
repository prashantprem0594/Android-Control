package com.appcontrol.parent.push;

import java.util.Base64;
import java.util.Map;

/** A Firebase hint is an opaque wake-up only, never call or authorization data. */
public final class ParentPushHint {
    private ParentPushHint() { }

    public static boolean accepts(Map<String, String> data, boolean hasNotification) {
        if (hasNotification || data == null || data.size() != 1) return false;
        String nonce = data.get("syncHint");
        if (nonce == null || !nonce.matches("[A-Za-z0-9_-]{22}")) return false;
        try {
            byte[] decoded = Base64.getUrlDecoder().decode(nonce);
            return decoded.length == 16 && Base64.getUrlEncoder().withoutPadding().encodeToString(decoded).equals(nonce);
        } catch (IllegalArgumentException rejected) {
            return false;
        }
    }
}
