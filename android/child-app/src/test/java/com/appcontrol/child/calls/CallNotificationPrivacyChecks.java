package com.appcontrol.child.calls;

public final class CallNotificationPrivacyChecks {
    private static void require(boolean value, String message) {
        if (!value) throw new AssertionError(message);
    }

    public static void main(String[] args) {
        String[] markers = {
            "SYNTHETIC_CONTACT_NAME_MARKER",
            "SYNTHETIC_PHONE_NUMBER_MARKER",
            "SYNTHETIC_CALL_DETAILS_MARKER"
        };
        String lockScreenText = CallNotificationPrivacy.lockScreenText();
        require(lockScreenText.equals("Unlock to view call details"), "lock-screen text changed unexpectedly");
        for (String marker : markers) {
            require(!lockScreenText.contains(marker), "lock-screen notification exposed caller details");
        }
        System.out.println("call-notification-privacy-checks-passed: caller details are redacted on the lock screen");
    }
}
