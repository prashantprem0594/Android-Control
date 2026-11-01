package com.appcontrol.transport;

import java.nio.charset.StandardCharsets;
import java.security.KeyFactory;
import java.security.Signature;
import java.security.spec.X509EncodedKeySpec;
import java.time.Duration;
import java.time.Instant;
import java.util.Arrays;
import java.util.Base64;
import java.util.HashSet;
import java.util.Iterator;
import java.util.Set;
import org.json.JSONArray;
import org.json.JSONObject;
import org.json.JSONTokener;

/** Verifies public release metadata only. It never downloads or installs an APK. */
public final class ReleaseManifest {
    private ReleaseManifest() {}
    private static void require(boolean value) { if (!value) throw new IllegalArgumentException("Invalid release metadata"); }
    private static void keys(JSONObject object, String... expected) {
        Set<String> actual = new HashSet<>(); Iterator<String> iterator = object.keys();
        while (iterator.hasNext()) actual.add(iterator.next());
        require(actual.equals(new HashSet<>(Arrays.asList(expected))));
    }
    private static String string(JSONObject object, String field) throws Exception {
        Object value = object.get(field); require(value instanceof String); return (String) value;
    }
    private static long integer(JSONObject object, String field) throws Exception {
        Object value = object.get(field); require(value instanceof Integer || value instanceof Long); return ((Number) value).longValue();
    }
    private static byte[] decode(String value, boolean url) {
        byte[] decoded = (url ? Base64.getUrlDecoder() : Base64.getDecoder()).decode(value);
        String canonical = (url ? Base64.getUrlEncoder().withoutPadding() : Base64.getEncoder()).encodeToString(decoded);
        require(canonical.equals(value)); return decoded;
    }
    private static JSONObject object(byte[] encoded) throws Exception {
        JSONTokener parser = new JSONTokener(new String(encoded, StandardCharsets.UTF_8));
        Object value = parser.nextValue(); require(value instanceof JSONObject && parser.nextClean() == 0);
        return (JSONObject) value;
    }
    public static JSONObject verify(byte[] encoded, String trustedPublicKey, String origin, String certificate,
                                    long minimumSequence, Instant now) {
        try {
            require(encoded.length > 0 && encoded.length <= 65536 && minimumSequence >= 0 && minimumSequence <= 2100000000L);
            require(certificate.matches("[a-f0-9]{64}") && ServerOrigin.require(origin, origin).toString().equals(origin));
            String host = ServerOrigin.require(origin, origin).getHost();
            require(host.length() <= 253 && host.matches(".*[a-z].*"));
            for (String label : host.split("\\.", -1)) require(label.length() >= 1 && label.length() <= 63 && label.matches("[a-z0-9](?:[a-z0-9-]*[a-z0-9])?"));
            JSONObject envelope = object(encoded);
            keys(envelope, "version", "payload", "signature"); require(integer(envelope, "version") == 1);
            byte[] payload = decode(string(envelope, "payload"), true), signature = decode(string(envelope, "signature"), true);
            require(payload.length <= 32768 && signature.length == 64);
            byte[] publicDER = decode(trustedPublicKey, false); require(publicDER.length == 44);
            Signature verifier = Signature.getInstance("Ed25519");
            verifier.initVerify(KeyFactory.getInstance("Ed25519").generatePublic(new X509EncodedKeySpec(publicDER)));
            verifier.update("app-control-release-manifest-v1\n".getBytes(StandardCharsets.UTF_8)); verifier.update(payload);
            require(verifier.verify(signature));
            JSONObject manifest = object(payload);
            keys(manifest, "version", "sequence", "issuedAt", "expiresAt", "origin", "apps");
            long sequence = integer(manifest, "sequence");
            require(integer(manifest, "version") == 1 && sequence >= 1 && sequence <= 2100000000L && sequence >= minimumSequence && string(manifest, "origin").equals(origin));
            Instant issued = Instant.parse(string(manifest, "issuedAt")), expires = Instant.parse(string(manifest, "expiresAt"));
            require(expires.isAfter(issued) && Duration.between(issued, expires).compareTo(Duration.ofDays(30)) <= 0 && now.isBefore(expires) && !issued.isAfter(now.plusSeconds(300)));
            JSONArray apps = manifest.getJSONArray("apps"); require(apps.length() == 2); Set<String> seen = new HashSet<>();
            for (int index = 0; index < apps.length(); index++) {
                JSONObject app = apps.getJSONObject(index);
                keys(app, "package", "versionCode", "versionName", "minSdk", "targetSdk", "url", "bytes", "sha256", "signingCertificateSHA256");
                String packageName = string(app, "package");
                require(packageName.equals("com.appcontrol.parent") || packageName.equals("com.appcontrol.child")); require(seen.add(packageName));
                long version = integer(app, "versionCode"), min = integer(app, "minSdk"), target = integer(app, "targetSdk"), bytes = integer(app, "bytes");
<<<<<<< HEAD
                require(version >= 1 && version <= 2100000000L && min >= (packageName.equals("com.appcontrol.child") ? 28 : 33) && target >= 33 && target >= min && target <= 100 && bytes >= 1 && bytes <= 256L * 1024 * 1024);
=======
                require(version >= 1 && version <= 2100000000L && min >= 33 && target >= min && target <= 100 && bytes >= 1 && bytes <= 256L * 1024 * 1024);
>>>>>>> origin/main
                require(string(app, "versionName").matches("[A-Za-z0-9][A-Za-z0-9._+\\-]{0,63}") && string(app, "sha256").matches("[a-f0-9]{64}") && string(app, "signingCertificateSHA256").equals(certificate));
                String name = packageName.equals("com.appcontrol.parent") ? "parent" : "child";
                require(string(app, "url").equals(origin + "/releases/" + sequence + "/" + name + "-release.apk"));
            }
            return manifest;
        } catch (Exception failure) { throw new IllegalArgumentException("Invalid release metadata"); }
    }
}
