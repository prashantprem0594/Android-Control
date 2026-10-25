package org.example.appcontroltelecomfixture;

import android.content.ComponentName;
import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.net.Uri;
import android.os.Binder;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.os.Process;
import android.telecom.DisconnectCause;
import android.telecom.PhoneAccount;
import android.telecom.PhoneAccountHandle;
import android.telecom.TelecomManager;
import java.util.Arrays;
import java.util.Collections;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicReference;

/** Controller accepts only the same-signing-key child on the guarded debug AVD. */
public final class FixtureProvider extends ContentProvider {
    static PhoneAccountHandle account(Context context) {
        return new PhoneAccountHandle(new ComponentName(context, FixtureConnectionService.class), "app-control-synthetic-fixture");
    }
    @Override public boolean onCreate() { return true; }
    @Override public Bundle call(String method, String arg, Bundle extras) {
        Context context = getContext();
        String[] packages = context.getPackageManager().getPackagesForUid(Binder.getCallingUid());
        if (!Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:") ||
            (context.getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) == 0 || packages == null ||
            !Arrays.asList(packages).contains("com.appcontrol.child") ||
            context.getPackageManager().checkSignatures(Binder.getCallingUid(), Process.myUid()) != PackageManager.SIGNATURE_MATCH)
            throw new SecurityException("Isolated fixture caller required");
        CountDownLatch done = new CountDownLatch(1); AtomicReference<Bundle> result = new AtomicReference<>();
        new Handler(Looper.getMainLooper()).post(() -> {
            Bundle response = new Bundle();
            try {
                TelecomManager telecom = context.getSystemService(TelecomManager.class);
                if ("prepare".equals(method)) {
                    if (FixtureConnectionService.current != null) throw new IllegalStateException();
                    telecom.registerPhoneAccount(PhoneAccount.builder(account(context), "App Control synthetic fixture")
                        .setCapabilities(PhoneAccount.CAPABILITY_CALL_PROVIDER).setSupportedUriSchemes(Collections.singletonList("sip")).build());
                    response.putString("result", "prepared");
                } else if ("incoming".equals(method)) {
                    if (FixtureConnectionService.current != null || FixtureConnectionService.expected) throw new IllegalStateException();
                    FixtureConnectionService.expected = true;
                    Bundle call = new Bundle(); call.putParcelable(TelecomManager.EXTRA_INCOMING_CALL_ADDRESS, Uri.fromParts("sip", "fixture.invalid", null));
                    telecom.addNewIncomingCall(account(context), call); response.putString("result", "requested");
                } else if ("state".equals(method)) {
                    response.putString("result", FixtureConnectionService.observation);
                } else if ("cleanup".equals(method)) {
                    FixtureConnectionService.expected = false; FixtureConnectionService.finish(DisconnectCause.LOCAL);
                    telecom.unregisterPhoneAccount(account(context)); response.putString("result", "cleaned");
                } else throw new IllegalArgumentException();
            } catch (Exception failure) { response.putString("result", "failed"); }
            result.set(response); done.countDown();
        });
        try { if (!done.await(10, TimeUnit.SECONDS)) throw new IllegalStateException("Fixture request timed out"); }
        catch (InterruptedException interrupted) { Thread.currentThread().interrupt(); throw new IllegalStateException("Fixture request interrupted"); }
        return result.get();
    }
    @Override public Cursor query(Uri uri, String[] projection, String selection, String[] arguments, String sort) { throw new UnsupportedOperationException(); }
    @Override public String getType(Uri uri) { return null; }
    @Override public Uri insert(Uri uri, ContentValues values) { throw new UnsupportedOperationException(); }
    @Override public int delete(Uri uri, String selection, String[] arguments) { throw new UnsupportedOperationException(); }
    @Override public int update(Uri uri, ContentValues values, String selection, String[] arguments) { throw new UnsupportedOperationException(); }
}
