package com.appcontrol.transport;

import android.os.Bundle;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.Base64;
import org.json.JSONObject;

/** Public randomly signed fixture only; no key custody, download or installation. */
public final class ReleaseManifestProbe {
    private static void require(boolean value) { if (!value) throw new IllegalStateException("Release verification invariant failed"); }
    private static void rejected(byte[] envelope, String publicKey, String origin, String certificate, long sequence, Instant now) {
        boolean refused = false;
        try { ReleaseManifest.verify(envelope, publicKey, origin, certificate, sequence, now); }
        catch (IllegalArgumentException expected) { refused = true; }
        require(refused);
    }
    public static Bundle run(Bundle arguments) {
        Bundle result = new Bundle(); String stage = "fixture_guard";
        try {
            String encoded = arguments.getString("fixture", ""); require(encoded.length() > 0 && encoded.length() <= 16000);
            JSONObject fixture = new JSONObject(new String(Base64.getDecoder().decode(encoded), StandardCharsets.UTF_8));
            require(fixture.getBoolean("fixtureOnly") && fixture.getString("origin").equals("https://release-manifest-fixture.example.org"));
            String publicKey = fixture.getString("publicKey"), origin = fixture.getString("origin"), certificate = fixture.getString("certificateSHA256");
            Instant now = Instant.parse(fixture.getString("now"));
            JSONObject envelope = fixture.getJSONObject("manifest"); byte[] raw = envelope.toString().getBytes(StandardCharsets.UTF_8);
            stage = "go_android_signature";
            JSONObject manifest = ReleaseManifest.verify(raw, publicKey, origin, certificate, 1, now);
            require(manifest.getLong("sequence") == 1 && manifest.getJSONArray("apps").length() == 2);
            stage = "trust_and_replay_negatives";
            rejected(raw, publicKey, "https://other.example.org", certificate, 1, now);
            rejected(raw, publicKey, origin, new String(new char[64]).replace('\0', 'c'), 1, now);
            rejected(raw, publicKey, origin, certificate, 2, now);
            rejected(raw, publicKey, origin, certificate, 1, Instant.parse(manifest.getString("expiresAt")));
            byte[] otherKey = Base64.getDecoder().decode(publicKey); otherKey[otherKey.length - 1] ^= 1;
            rejected(raw, Base64.getEncoder().encodeToString(otherKey), origin, certificate, 1, now);
            stage = "tamper_and_format_negatives";
            JSONObject changed = new JSONObject(envelope.toString());
            byte[] payload = Base64.getUrlDecoder().decode(changed.getString("payload")); payload[0] ^= 1;
            changed.put("payload", Base64.getUrlEncoder().withoutPadding().encodeToString(payload));
            rejected(changed.toString().getBytes(StandardCharsets.UTF_8), publicKey, origin, certificate, 1, now);
            changed = new JSONObject(envelope.toString()).put("unexpected", true);
            rejected(changed.toString().getBytes(StandardCharsets.UTF_8), publicKey, origin, certificate, 1, now);
            rejected((envelope.toString()+"{}").getBytes(StandardCharsets.UTF_8), publicKey, origin, certificate, 1, now);
            result.putString("release_manifest_probe", "passed");
            result.putString("stream", "Go-signed Ed25519 release metadata verified on Android; wrong trust, replay, expiry, tampering and trailing data refused\n");
        } catch (Exception failure) {
            result.putString("release_manifest_probe", "failed"); result.putString("observation", stage);
        }
        return result;
    }
}
