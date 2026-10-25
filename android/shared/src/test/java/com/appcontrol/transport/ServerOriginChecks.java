package com.appcontrol.transport;

public final class ServerOriginChecks {
    public static void main(String[] args) {
        String origin = "https://control.example.test";
        check(ServerOrigin.require(" https://CONTROL.example.test:443/ ", origin).toString().equals(origin));
        check(ServerOrigin.require("https://control.example.test:8443", "https://CONTROL.example.test:8443/").getPort() == 8443);
        check(ServerOrigin.require("https://other.example.test", "").getHost().equals("other.example.test"));
        for (String input : new String[]{"http://control.example.test", "https://other.example.test", "https://control.example.test.attacker.test",
                "https://user@control.example.test", "https://control.example.test?token=secret", "https://control.example.test#fragment",
                "https://control.example.test/v1", "https://control.example.test/../", "https://control.example.test/%2f",
                "https://control.example.test:8443", "https://control.example.test:0", "https://control.example.test:65536",
                "https://control.example.test.", "https://control.example.test%2f.attacker.test", "https://control.example.test\\@other.test",
                "https:///control.example.test", "https:control.example.test", "", null}) {
            try { ServerOrigin.require(input, origin); throw new AssertionError("Unsafe or mismatched origin accepted"); }
            catch (IllegalArgumentException expected) { }
        }
        for (String input : new String[]{"http://example.test", "https://example.test/path", "https://user:pass@example.test", "https://example.test?x", "https://example.test#x"}) {
            try { ServerOrigin.require(input, ""); throw new AssertionError("Invalid development origin accepted"); }
            catch (IllegalArgumentException expected) { }
        }
        System.out.println("Server origin checks passed: canonical hosts/ports, deployment binding and invalid-origin refusal.");
    }
    private static void check(boolean value) { if (!value) throw new AssertionError("Origin canonicalization failed"); }
}
