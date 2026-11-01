package com.appcontrol.child;

import android.content.Context;
import android.content.ContextWrapper;
import android.app.admin.DevicePolicyManager;
import android.content.SharedPreferences;
import android.os.Bundle;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.util.UUID;
import javax.net.ssl.HttpsURLConnection;
import org.json.JSONArray;
import org.json.JSONObject;
import com.appcontrol.transport.AcceptanceHttps;
import com.appcontrol.child.network.DeviceEnrollmentClient;
import com.appcontrol.child.enrollment.DeviceIdentity;
import com.appcontrol.child.calls.CallContactStore;
import java.security.KeyStore;
import java.util.HashSet;
import java.util.Arrays;

/** Exercises real HTTPS enrollment and protocol calls using only the disposable acceptance app identity. */
public final class LiveServerHttpsProbe {
    private static final String EMAIL = "acceptance-owner@example.test";
    private static final String PASSWORD = "acceptance-fixture-password-not-secret";
    private static final String DEVICE_KEY_ALIAS = "app-control-device-v1";
    private static String origin;
    private static javax.net.ssl.SSLSocketFactory sockets;

    private LiveServerHttpsProbe() {}

    private static final class HttpStatusFailure extends Exception {
        final int status;
        HttpStatusFailure(int status) { this.status = status; }
    }

    public static Bundle run(Context base) {
        String contactPreferences = "fixture-live-contacts-" + UUID.randomUUID();
        Context context = new ContextWrapper(base) {
            @Override public Context getApplicationContext() { return this; }
            @Override public SharedPreferences getSharedPreferences(String name, int mode) {
                return base.getSharedPreferences("call-contact-cache".equals(name) ? contactPreferences : name, mode);
            }
        };
        Bundle result = new Bundle();
        String phase = "guard";
        boolean enrollmentPrefsOwned = false;
        boolean identityKeyOwned = false;
        boolean contactKeyOwned = false;
        boolean contactFailureEnabled = false;
        javax.net.ssl.SSLSocketFactory previousSocketFactory = HttpsURLConnection.getDefaultSSLSocketFactory();
        boolean acceptanceSocketFactoryInstalled = false;
        try {
            if (!BuildConfig.DEBUG || BuildConfig.SERVER_ORIGIN.isEmpty()) throw new IllegalStateException();
            if (((DevicePolicyManager) context.getSystemService(Context.DEVICE_POLICY_SERVICE))
                    .isDeviceOwnerApp(context.getPackageName())) throw new IllegalStateException();
            origin = BuildConfig.SERVER_ORIGIN;
            URL parsed = new URL(origin);
            if (!"https".equals(parsed.getProtocol()) || !("localhost".equals(parsed.getHost()) || "10.0.2.2".equals(parsed.getHost()))) throw new IllegalStateException();
            sockets = AcceptanceHttps.socketFactory();
            HttpsURLConnection.setDefaultSSLSocketFactory(sockets);
            acceptanceSocketFactoryInstalled = true;
            phase = "parent-account-register";
            request("POST", "/v1/auth/register", null,
                new JSONObject().put("email", EMAIL).put("password", PASSWORD), null, 201, 409);
            phase = "parent-account-login";
            JSONObject owner = request("POST", "/v1/auth/login", null,
                new JSONObject().put("email", EMAIL).put("password", PASSWORD), null, 200);
            String parentToken = owner.getJSONObject("tokens").getString("accessToken");
            phase = "pairing-code";
            JSONObject code = request("POST", "/v1/pairing/codes", parentToken, new JSONObject(), null, 201);
            SharedPreferences enrollmentState = context.getSharedPreferences("enrollment", Context.MODE_PRIVATE);
            if (!enrollmentState.getAll().isEmpty()) throw new IllegalStateException();
            KeyStore androidKeys = KeyStore.getInstance("AndroidKeyStore");
            androidKeys.load(null);
            contactKeyOwned = !androidKeys.containsAlias("appcontrol.callcontacts.v1");
            if (androidKeys.containsAlias(DEVICE_KEY_ALIAS)) throw new IllegalStateException();
            enrollmentPrefsOwned = true;
            identityKeyOwned = true;
            DeviceEnrollmentClient enrollmentClient = new DeviceEnrollmentClient(context);
            phase = "configured-origin-refusal";
            boolean foreignOriginRejected = false;
            try { enrollmentClient.enroll("https://127.0.0.1:1", "unused-fixture-code", "Rejected fixture"); }
            catch (IllegalArgumentException expected) { foreignOriginRejected = true; }
            if (!foreignOriginRejected || androidKeys.containsAlias(DEVICE_KEY_ALIAS) || !enrollmentState.getAll().isEmpty()) {
                throw new IllegalStateException();
            }
            phase = "android-keystore-enrollment";
            String deviceId = enrollmentClient.enroll(origin, code.getString("code"), "Acceptance fixture");
            if (!deviceId.matches("dev_[A-Za-z0-9_-]{22}") || !deviceId.equals(enrollmentState.getString("deviceId", null)) ||
                !origin.equals(enrollmentState.getString("server", null)) || enrollmentState.getAll().size() != 2 ||
                !enrollmentState.getAll().keySet().equals(new HashSet<>(Arrays.asList("server", "deviceId"))) || !androidKeys.containsAlias(DEVICE_KEY_ALIAS)) {
                throw new IllegalStateException();
            }
            phase = "pairing-code-reuse";
            request("POST", "/v1/enrollments", null, new JSONObject()
                .put("code", code.getString("code")).put("displayName", "Acceptance replay")
                .put("publicKey", new DeviceIdentity().publicKey()).put("keyAlgorithm", "ECDSA_P256_SHA256"), null, 400);
            phase = "device-auth";
            request("GET", "/v1/device/commands", null, null, null, 401);
            phase = "android-keystore-authentication";
            String deviceToken = enrollmentClient.authenticate(new kotlin.jvm.functions.Function0<Boolean>() {
                @Override public Boolean invoke() { return false; }
            });
            phase = "device-auth-authorization";
            JSONObject identity = request("GET", "/v1/device-auth/me", deviceToken, null, null, 200);
            if (!deviceId.equals(identity.getString("deviceId"))) throw new IllegalStateException();
            request("GET", "/v1/device/policy", null, null, null, 401);
            phase = "command-and-policy-setup";
            JSONObject queued = request("POST", "/v1/devices/" + deviceId + "/commands", parentToken,
                new JSONObject().put("type", "LOCK_DEVICE").put("ttlSeconds", 60),
                UUID.randomUUID().toString(), 201);
            request("PUT", "/v1/devices/" + deviceId + "/policy", parentToken,
                new JSONObject().put("expectedVersion", 0).put("internetPaused", false)
                    .put("blockedPackages", new JSONArray()), null, 200);
            phase = "parent-contact-create";
            updateContacts(deviceId, parentToken, 0, "Fixture parent", "+12025550123");
            phase = "production-command-sync";
            com.appcontrol.child.commands.CommandSync.Summary summary =
                new com.appcontrol.child.commands.CommandSync(context).sync(
                    new kotlin.jvm.functions.Function0<Boolean>() {
                        @Override public Boolean invoke() { return false; }
                    });
            if (summary.getAcknowledged() != 1 || summary.getDownloadedPolicyVersion() != 1) {
                throw new IllegalStateException();
            }
            phase = "synced-contact-cache";
            requireContact(context, 1, "Fixture parent", "+12025550123");
            for (Object sealed : context.getSharedPreferences("call-contact-cache", Context.MODE_PRIVATE).getAll().values()) {
                String raw = String.valueOf(sealed);
                if (raw.contains("Fixture parent") || raw.contains("12025550123")) throw new IllegalStateException();
            }
            phase = "parent-contact-replacement";
            updateContacts(deviceId, parentToken, 1, "Updated fixture", "+12025550124");
            phase = "contact-service-unavailable";
            request("POST", "/__acceptance/contacts-unavailable", null, null, null, 204);
            contactFailureEnabled = true;
            request("GET", "/v1/device/call-contacts", deviceToken, null, null, 503);
            java.util.Map<String, ?> cachedBeforeFailure = new java.util.HashMap<>(
                context.getSharedPreferences("call-contact-cache", Context.MODE_PRIVATE).getAll());
            JSONObject outageCommand = request("POST", "/v1/devices/" + deviceId + "/commands", parentToken,
                new JSONObject().put("type", "LOCK_DEVICE").put("ttlSeconds", 60),
                UUID.randomUUID().toString(), 201);
            com.appcontrol.child.commands.CommandSync.Summary outageSummary = sync(context);
            if (outageSummary.getAcknowledged() != 1 || outageSummary.getDownloadedPolicyVersion() != 1 ||
                !cachedBeforeFailure.equals(context.getSharedPreferences("call-contact-cache", Context.MODE_PRIVATE).getAll())) {
                throw new IllegalStateException();
            }
            requireContact(context, 1, "Fixture parent", "+12025550123");
            JSONObject outageOutcome = request("GET", "/v1/devices/" + deviceId + "/commands/" +
                outageCommand.getString("commandId"), parentToken, null, null, 200);
            if (!"failed".equals(outageOutcome.getString("status"))) throw new IllegalStateException();
            phase = "contact-service-recovery";
            request("POST", "/__acceptance/contacts-available", null, null, null, 204);
            contactFailureEnabled = false;
            sync(context);
            requireContact(context, 2, "Updated fixture", "+12025550124");
            phase = "parent-contact-clear";
            request("PUT", "/v1/devices/" + deviceId + "/call-contacts", parentToken,
                new JSONObject().put("expectedVersion", 2).put("contacts", new JSONArray()), null, 200);
            sync(context);
            CallContactStore.Snapshot cleared = new CallContactStore(context).load();
            if (cleared.getVersion() != 3 || !cleared.getContacts().isEmpty()) throw new IllegalStateException();
            phase = "command-policy-outcome-inspection";
            String commandId = queued.getString("commandId");
            JSONObject acknowledged = request("GET", "/v1/devices/" + deviceId + "/commands/" + commandId,
                parentToken, null, null, 200);
            if (!"failed".equals(acknowledged.getString("status"))) throw new IllegalStateException();
            JSONObject parentPolicy = request("GET", "/v1/devices/" + deviceId + "/policy", parentToken, null, null, 200);
            JSONObject report = parentPolicy.getJSONObject("reported");
            if (report.getLong("version") != 1 || !"failed".equals(report.getString("result")) ||
                !"device_owner_required".equals(report.getString("reason"))) {
                throw new IllegalStateException();
            }
            phase = "pre-revocation-session-valid";
            JSONObject stillAuthenticated = request("GET", "/v1/device-auth/me", deviceToken, null, null, 200);
            if (!deviceId.equals(stillAuthenticated.getString("deviceId"))) throw new IllegalStateException();
            phase = "parent-device-revocation";
            request("POST", "/v1/devices/" + deviceId + "/revoke", parentToken,
                new JSONObject(), null, 204);
            phase = "revoked-device-session-rejection";
            request("GET", "/v1/device-auth/me", deviceToken, null, null, 401);
            request("GET", "/v1/device/commands", deviceToken, null, null, 401);
            phase = "parent-token-revocation";
            request("POST", "/v1/auth/logout", parentToken, new JSONObject(), null, 204);
            result.putString("server_https_child_probe", "passed");
            result.putString("server_https_device_identity_probe", "passed");
            result.putString("server_https_production_enrollment_probe", "passed");
            result.putString("server_https_contact_sync_probe", "passed");
            result.putString("server_https_contact_recovery_probe", "passed");
        } catch (Exception failure) {
            result.putString("server_https_child_probe", "failed");
            result.putString("server_https_child_phase", phase);
            if (failure instanceof HttpStatusFailure) {
                result.putInt("server_https_child_http_status", ((HttpStatusFailure) failure).status);
            } else {
                result.putString("server_https_child_failure", failure.getClass().getSimpleName());
            }
        } finally {
            Exception cleanupFailure = null;
            if (contactFailureEnabled) {
                try { request("POST", "/__acceptance/contacts-available", null, null, null, 204); }
                catch (Exception failure) { cleanupFailure = failure; }
            }
            if (acceptanceSocketFactoryInstalled) {
                try { HttpsURLConnection.setDefaultSSLSocketFactory(previousSocketFactory); }
                catch (Exception failure) { cleanupFailure = failure; }
            }
            try {
                if (!base.deleteSharedPreferences(contactPreferences)) throw new IllegalStateException();
                if (contactKeyOwned) {
                    KeyStore keys = KeyStore.getInstance("AndroidKeyStore");
                    keys.load(null);
                    keys.deleteEntry("appcontrol.callcontacts.v1");
                    if (keys.containsAlias("appcontrol.callcontacts.v1")) throw new IllegalStateException();
                }
            } catch (Exception failure) { cleanupFailure = failure; }
            try {
                if (enrollmentPrefsOwned && !context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).edit().clear().commit()) throw new IllegalStateException();
            } catch (Exception failure) { cleanupFailure = failure; }
            try {
                if (identityKeyOwned) {
                    KeyStore keys = KeyStore.getInstance("AndroidKeyStore");
                    keys.load(null);
                    if (keys.containsAlias(DEVICE_KEY_ALIAS)) keys.deleteEntry(DEVICE_KEY_ALIAS);
                    if (keys.containsAlias(DEVICE_KEY_ALIAS)) throw new IllegalStateException();
                }
            } catch (Exception failure) { cleanupFailure = failure; }
            if (cleanupFailure != null) {
                result.putString("server_https_child_probe", "failed");
                result.putString("server_https_child_phase", "acceptance-keystore-cleanup");
                result.putString("server_https_child_failure", cleanupFailure.getClass().getSimpleName());
                result.remove("server_https_device_identity_probe");
                result.remove("server_https_contact_sync_probe");
                result.remove("server_https_contact_recovery_probe");
            }
        }
        return result;
    }

    private static com.appcontrol.child.commands.CommandSync.Summary sync(Context context) {
        return new com.appcontrol.child.commands.CommandSync(context).sync(
            new kotlin.jvm.functions.Function0<Boolean>() {
                @Override public Boolean invoke() { return false; }
            });
    }

    private static void updateContacts(String deviceId, String token, long version, String label, String number) throws Exception {
        request("PUT", "/v1/devices/" + deviceId + "/call-contacts", token,
            new JSONObject().put("expectedVersion", version).put("contacts", new JSONArray().put(
                new JSONObject().put("label", label).put("number", number).put("isParent", true))), null, 200);
    }

    private static void requireContact(Context context, long version, String label, String number) {
        CallContactStore.Snapshot snapshot = new CallContactStore(context).load();
        if (snapshot.getVersion() != version || snapshot.getContacts().size() != 1) throw new IllegalStateException();
        CallContactStore.Contact contact = snapshot.getContacts().get(0);
        if (!label.equals(contact.getLabel()) || !number.equals(contact.getNumber()) || !contact.isParent()) {
            throw new IllegalStateException();
        }
    }

    private static JSONObject request(String method, String path, String bearer, JSONObject body,
            String idempotencyKey, int expected, int... alternatives) throws Exception {
        HttpsURLConnection connection = (HttpsURLConnection) new URL(origin + path).openConnection();
        connection.setSSLSocketFactory(sockets);
        connection.setRequestMethod(method);
        connection.setInstanceFollowRedirects(false);
        connection.setConnectTimeout(10000);
        connection.setReadTimeout(10000);
        connection.setUseCaches(false);
        connection.setRequestProperty("Accept", "application/json");
        if (bearer != null) connection.setRequestProperty("Authorization", "Bearer " + bearer);
        if (idempotencyKey != null) connection.setRequestProperty("Idempotency-Key", idempotencyKey);
        try {
            if (body != null) {
                byte[] bytes = body.toString().getBytes(StandardCharsets.UTF_8);
                connection.setDoOutput(true);
                connection.setRequestProperty("Content-Type", "application/json");
                connection.setFixedLengthStreamingMode(bytes.length);
                try (OutputStream output = connection.getOutputStream()) { output.write(bytes); }
            }
            int status = connection.getResponseCode();
            boolean accepted = status == expected;
            for (int alternative : alternatives) accepted |= status == alternative;
            if (!accepted) throw new HttpStatusFailure(status);
            InputStream stream = status >= 400 ? connection.getErrorStream() : connection.getInputStream();
            if (stream == null || status == 204) return new JSONObject().put("_status", status);
            byte[] bytes = readBounded(stream);
            return new JSONObject(new String(bytes, StandardCharsets.UTF_8)).put("_status", status);
        } finally {
            connection.disconnect();
        }
    }

    private static byte[] readBounded(InputStream stream) throws Exception {
        try (InputStream input = stream; ByteArrayOutputStream output = new ByteArrayOutputStream()) {
            byte[] buffer = new byte[4096];
            int total = 0, count;
            while ((count = input.read(buffer)) != -1) {
                total += count;
                if (total > 1024 * 1024) throw new IllegalStateException();
                output.write(buffer, 0, count);
            }
            return output.toByteArray();
        }
    }
}
