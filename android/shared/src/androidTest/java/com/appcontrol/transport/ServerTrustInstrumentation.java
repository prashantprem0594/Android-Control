package com.appcontrol.transport;

import android.app.Activity;
import android.app.Instrumentation;
import android.os.Bundle;
import java.net.URL;
import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLHandshakeException;
import javax.net.ssl.SSLPeerUnverifiedException;

/** Credential-free platform TLS probe, packaged only in the test APK. */
public final class ServerTrustInstrumentation extends Instrumentation {
    private Bundle arguments;

    @Override public void onCreate(Bundle supplied) {
        super.onCreate(supplied);
        arguments = supplied == null ? new Bundle() : new Bundle(supplied);
        start();
    }

    @Override public void onStart() {
        if ("contact_cache".equals(arguments.getString("probe"))) {
            Bundle contactResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                contactResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.child.CallContactCacheProbe")
                    .getMethod("run", android.content.Context.class).invoke(null, getTargetContext());
            } catch (Exception ignored) { contactResult.putString("contact_cache_probe", "failed"); }
            finish("passed".equals(contactResult.getString("contact_cache_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, contactResult);
            return;
        }
        if ("release_artifact".equals(arguments.getString("probe"))) {
            Bundle artifactResult = ReleaseArtifactProbe.run(getTargetContext(), getContext());
            finish("passed".equals(artifactResult.getString("release_artifact_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, artifactResult);
            return;
        }
        if ("parent_persistence".equals(arguments.getString("probe"))) {
            Bundle persistenceResult = new Bundle();
            try {
                if (!"com.appcontrol.parent".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                persistenceResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.parent.ParentPersistenceProbe")
                    .getMethod("run", android.content.Context.class, String.class)
                    .invoke(null, getTargetContext(), arguments.getString("phase", "restart"));
            } catch (Exception failure) { persistenceResult.putString("parent_persistence_probe", "failed"); }
            String state = persistenceResult.getString("parent_persistence_probe", "failed");
            finish(state.equals("prepared") || state.equals("passed") || state.equals("cleaned") ? Activity.RESULT_OK : Activity.RESULT_CANCELED, persistenceResult);
            return;
        }
        if ("telecom_fixture".equals(arguments.getString("probe"))) {
            Bundle telecomResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                telecomResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.child.TelecomFixtureProbe")
                    .getMethod("run", android.content.Context.class, Instrumentation.class, String.class)
                    .invoke(null, getTargetContext(), this, arguments.getString("phase", "run"));
            } catch (Exception failure) { telecomResult.putString("telecom_probe", "failed"); }
            String state = telecomResult.getString("telecom_probe", "failed");
            finish(state.equals("prepared") || state.equals("passed") || state.equals("cleaned") ? Activity.RESULT_OK : Activity.RESULT_CANCELED, telecomResult);
            return;
        }
        if ("release_manifest".equals(arguments.getString("probe"))) {
            Bundle releaseResult = ReleaseManifestProbe.run(arguments);
            finish("passed".equals(releaseResult.getString("release_manifest_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, releaseResult);
            return;
        }
        if ("policy_upgrade".equals(arguments.getString("probe"))) {
            Bundle upgradeResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                upgradeResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.child.PolicyUpgradeProbe")
                    .getMethod("run", android.content.Context.class, String.class)
                    .invoke(null, getTargetContext(), arguments.getString("phase", "verify"));
            } catch (Exception failure) { upgradeResult.putString("policy_upgrade_probe", "failed"); }
            String state = upgradeResult.getString("policy_upgrade_probe", "failed");
            finish(state.equals("prepared") || state.equals("passed") || state.equals("cleaned") ? Activity.RESULT_OK : Activity.RESULT_CANCELED, upgradeResult);
            return;
        }
        if ("parent_client".equals(arguments.getString("probe"))) {
            Bundle parentResult = new Bundle();
            try {
                if (!"com.appcontrol.parent".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                parentResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.parent.ParentClientProbe")
                    .getMethod("run", android.content.Context.class).invoke(null, getTargetContext());
            } catch (Exception failure) {
                parentResult.putString("parent_client_probe", "failed");
            }
            finish("passed".equals(parentResult.getString("parent_client_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, parentResult);
            return;
        }
        if ("parent_session".equals(arguments.getString("probe"))) {
            Bundle parentResult = new Bundle();
            try {
                if (!"com.appcontrol.parent".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                parentResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.parent.ParentSessionProbe")
                    .getMethod("run", android.content.Context.class).invoke(null, getTargetContext());
            } catch (Exception failure) {
                parentResult.putString("parent_session_probe", "failed");
            }
            finish("passed".equals(parentResult.getString("parent_session_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, parentResult);
            return;
        }
        if ("schedule_boundary".equals(arguments.getString("probe"))) {
            Bundle scheduleResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                scheduleResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.child.ScheduleBoundaryProbe")
                    .getMethod("run", android.content.Context.class, Instrumentation.class).invoke(null, getTargetContext(), this);
            } catch (Exception failure) {
                scheduleResult.putString("schedule_probe", "failed");
                scheduleResult.putString("stream", "Android schedule probe setup failed\n");
            }
            finish("passed".equals(scheduleResult.getString("schedule_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, scheduleResult);
            return;
        }
        if ("policy_reboot".equals(arguments.getString("probe"))) {
            Bundle rebootResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                rebootResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.child.PolicyRebootProbe")
                    .getMethod("run", android.content.Context.class, String.class)
                    .invoke(null, getTargetContext(), arguments.getString("phase", "verify"));
            } catch (Exception failure) {
                rebootResult.putString("policy_reboot_probe", "failed");
                rebootResult.putString("stream", "Android policy reboot probe setup failed\n");
            }
            String state = rebootResult.getString("policy_reboot_probe", "failed");
            finish(state.equals("prepared") || state.equals("passed") || state.equals("cleaned") ? Activity.RESULT_OK : Activity.RESULT_CANCELED, rebootResult);
            return;
        }
        if ("package_policy".equals(arguments.getString("probe"))) {
            Bundle packageResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                packageResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.child.PackagePolicyProbe")
                    .getMethod("run", android.content.Context.class).invoke(null, getTargetContext());
            } catch (Exception failure) {
                packageResult.putString("package_probe", "failed");
                packageResult.putString("stream", "Android package policy probe setup failed\n");
            }
            finish("passed".equals(packageResult.getString("package_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, packageResult);
            return;
        }
        if ("journal_process".equals(arguments.getString("probe"))) {
            Bundle journalResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                journalResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.child.JournalProcessProbe")
                    .getMethod("run", android.content.Context.class).invoke(null, getTargetContext());
            } catch (Exception failure) {
                journalResult.putString("journal_probe", "failed");
                journalResult.putString("stream", "Android journal probe setup failed\n");
            }
            finish("passed".equals(journalResult.getString("journal_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, journalResult);
            return;
        }
        if ("request_control".equals(arguments.getString("probe"))) {
            Bundle requestResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                requestResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.child.RequestControlProbe")
                    .getMethod("run").invoke(null);
            } catch (Exception failure) {
                requestResult.putString("request_probe", "failed");
                requestResult.putString("stream", "Android request probe setup failed\n");
            }
            finish("passed".equals(requestResult.getString("request_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, requestResult);
            return;
        }
        if ("work_queue".equals(arguments.getString("probe"))) {
            Bundle queueResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                queueResult = (Bundle) getContext().getClassLoader().loadClass("com.appcontrol.child.WorkQueueProbe")
                    .getMethod("run", android.content.Context.class).invoke(null, getTargetContext());
            } catch (Exception failure) {
                queueResult.putString("queue_probe", "failed");
                queueResult.putString("stream", "Android queue probe setup failed\n");
            }
            finish("passed".equals(queueResult.getString("queue_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, queueResult);
            return;
        }
        if ("identity".equals(arguments.getString("probe"))) {
            Bundle identityResult = new Bundle();
            try {
                if (!"com.appcontrol.child".equals(getTargetContext().getPackageName())) throw new IllegalArgumentException();
                identityResult = (Bundle) getContext().getClassLoader()
                    .loadClass("com.appcontrol.child.DeviceIdentityProbe").getMethod("run").invoke(null);
            } catch (Exception failure) {
                identityResult.putString("identity_probe", "failed");
                identityResult.putString("stream", "Android Keystore identity probe: failed\n");
            }
            finish("passed".equals(identityResult.getString("identity_probe")) ? Activity.RESULT_OK : Activity.RESULT_CANCELED, identityResult);
            return;
        }
        Bundle result = new Bundle();
        HttpsURLConnection connection = null;
        String outcome = "setup_failure";
        boolean passed = false;
        try {
            String expected = arguments.getString("expect", "");
            if (!expected.equals("success") && !expected.equals("tls_failure")) {
                throw new IllegalArgumentException();
            }
            String packageName = getTargetContext().getPackageName();
            Class<?> build = getTargetContext().getClassLoader().loadClass(packageName + ".BuildConfig");
            String origin = (String) build.getField("SERVER_ORIGIN").get(null);
            if (origin == null || origin.isEmpty()) throw new IllegalArgumentException();
            // Use only the compiled deployment origin. No arbitrary URL, user
            // credentials, trust override or redirect is accepted by this probe.
            URL endpoint = new URL(origin + "/healthz");
            connection = (HttpsURLConnection) endpoint.openConnection();
            connection.setInstanceFollowRedirects(false);
            connection.setConnectTimeout(10000);
            connection.setReadTimeout(10000);
            connection.setUseCaches(false);
            // Each instrumentation invocation is a fresh process, avoiding TLS
            // session/connection reuse across server certificate changes.
            try {
                int status = connection.getResponseCode();
                outcome = status == 200 ? "https_200" : "unexpected_http_status";
                passed = expected.equals("success") && status == 200;
            } catch (SSLHandshakeException | SSLPeerUnverifiedException rejected) {
                outcome = "tls_handshake_rejected";
                passed = expected.equals("tls_failure");
            }
        } catch (Exception failure) {
            // DNS errors, timeouts, reflection/setup errors and other transport
            // failures cannot satisfy an expected certificate rejection.
            outcome = "setup_or_transport_failure";
        } finally {
            if (connection != null) connection.disconnect();
        }
        result.putString("trust_probe", passed ? "passed" : "failed");
        result.putString("observation", outcome);
        result.putString("stream", "Server trust probe: " + (passed ? "passed" : "failed") + " (" + outcome + ")\n");
        finish(passed ? Activity.RESULT_OK : Activity.RESULT_CANCELED, result);
    }
}
