package com.appcontrol.transport;

import android.content.pm.ApplicationInfo;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.content.pm.Signature;
import android.os.Build;
import java.io.File;
import java.io.FileInputStream;
import java.security.MessageDigest;
import java.time.Instant;
import org.json.JSONArray;
import org.json.JSONObject;

/** Inspects a privately staged APK against independently trusted signed metadata.
 * No downloading, installation or authorization is performed. The installer must
 * consume the same verified bytes and still performs Android signature checks.
 */
public final class ReleaseArtifact {
    private ReleaseArtifact() { }
    private static void require(boolean value) { if (!value) throw new IllegalArgumentException("Invalid release artifact"); }
    private static String hex(byte[] bytes) {
        StringBuilder text = new StringBuilder();
        for (byte value : bytes) text.append(String.format(java.util.Locale.ROOT, "%02x", value & 255));
        return text.toString();
    }
    public static PackageInfo inspect(PackageManager manager, File apk, byte[] envelope, String publicKey,
                                      String origin, String certificate, long minimumSequence,
                                      String packageName, Instant now) {
        String stage = "metadata";
        try {
            JSONObject manifest = ReleaseManifest.verify(envelope, publicKey, origin, certificate, minimumSequence, now);
            JSONObject expected = null; JSONArray apps = manifest.getJSONArray("apps");
            for (int i = 0; i < apps.length(); i++) if (apps.getJSONObject(i).getString("package").equals(packageName)) expected = apps.getJSONObject(i);
            stage = "file";
            // Android's private storage ancestors may themselves be system
            // symlinks (/data/user/0). Reject a symlink at the artifact itself.
            require(expected != null && android.system.OsConstants.S_ISREG(android.system.Os.lstat(apk.getAbsolutePath()).st_mode));
            long size = expected.getLong("bytes"); require(apk.length() == size);
            stage = "bytes";
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            try (FileInputStream stream = new FileInputStream(apk)) {
                byte[] buffer = new byte[65536]; long total = 0; int count;
                while ((count = stream.read(buffer)) != -1) {
                    require(!Thread.currentThread().isInterrupted());
                    total += count; require(total <= size); digest.update(buffer, 0, count);
                }
                require(total == size);
            }
            require(hex(digest.digest()).equals(expected.getString("sha256")));
            stage = "archive";
            PackageInfo actual = manager.getPackageArchiveInfo(apk.getAbsolutePath(), PackageManager.PackageInfoFlags.of(PackageManager.GET_SIGNING_CERTIFICATES));
            require(actual != null && actual.applicationInfo != null && actual.signingInfo != null);
            stage = "package";
            require(packageName.equals(actual.packageName) && actual.getLongVersionCode() == expected.getLong("versionCode") &&
                expected.getString("versionName").equals(actual.versionName));
            stage = "sdk";
            require(actual.applicationInfo.minSdkVersion == expected.getInt("minSdk") && actual.applicationInfo.targetSdkVersion == expected.getInt("targetSdk"));
            require(actual.applicationInfo.minSdkVersion <= Build.VERSION.SDK_INT);
            stage = "production";
            require((actual.applicationInfo.flags & (ApplicationInfo.FLAG_DEBUGGABLE | ApplicationInfo.FLAG_TEST_ONLY)) == 0);
            require(actual.splitNames == null || actual.splitNames.length == 0);
            stage = "signer";
            require(!actual.signingInfo.hasMultipleSigners());
            Signature[] signers = actual.signingInfo.getApkContentsSigners();
            require(signers != null && signers.length == 1 && hex(MessageDigest.getInstance("SHA-256").digest(signers[0].toByteArray())).equals(certificate));
            return actual;
        } catch (Exception rejected) { throw new IllegalArgumentException("Invalid release artifact: " + stage); }
    }
    /** Conservative same-key upgrade only; no downgrade or signer-rotation bypass. */
    public static void requireUpgrade(PackageInfo installed, PackageInfo candidate) {
        try {
            require(installed != null && candidate != null && installed.packageName.equals(candidate.packageName));
            require(candidate.getLongVersionCode() > installed.getLongVersionCode());
            require(installed.signingInfo != null && candidate.signingInfo != null && !installed.signingInfo.hasMultipleSigners() && !candidate.signingInfo.hasMultipleSigners());
            Signature[] before = installed.signingInfo.getApkContentsSigners(), after = candidate.signingInfo.getApkContentsSigners();
            require(before != null && after != null && before.length == 1 && after.length == 1 && before[0].equals(after[0]));
        } catch (Exception rejected) { throw new IllegalArgumentException("Incompatible release upgrade"); }
    }
}
