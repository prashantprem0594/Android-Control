package com.appcontrol.transport;

import android.content.Context;
import java.io.File;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.net.URL;
import java.time.Instant;
import java.util.function.BooleanSupplier;
import java.util.function.Supplier;
import javax.net.ssl.HttpsURLConnection;
import org.json.JSONArray;
import org.json.JSONObject;

/** Public HTTPS artifact staging only. No bearer credentials or installation. */
public final class ReleaseDownload {
    private ReleaseDownload() { }
    interface Connections { HttpsURLConnection open(URL url) throws Exception; }
    public static File stage(Context context, byte[] envelope, String publicKey, String origin,
                             String certificate, long minimumSequence, BooleanSupplier stopped) {
        return stage(context, envelope, publicKey, origin, certificate, minimumSequence, stopped,
            url -> (HttpsURLConnection) url.openConnection(), Instant::now);
    }
    static File stage(Context context, byte[] envelope, String publicKey, String origin,
                      String certificate, long minimumSequence, BooleanSupplier stopped,
                      Connections connections, Supplier<Instant> clock) {
        File candidate = null; boolean completed = false;
        HttpsURLConnection connection = null; RequestDeadline deadline = null;
        try {
            Thread owner = Thread.currentThread();
            BooleanSupplier cancelled = () -> owner.isInterrupted() || stopped.getAsBoolean();
            JSONObject manifest = ReleaseManifest.verify(envelope, publicKey, origin, certificate, minimumSequence, clock.get());
            JSONObject artifact = null; JSONArray apps = manifest.getJSONArray("apps");
            for (int i=0;i<apps.length();i++) if (context.getPackageName().equals(apps.getJSONObject(i).getString("package"))) artifact=apps.getJSONObject(i);
            if (artifact == null || cancelled.getAsBoolean()) throw new IllegalArgumentException();
            long expected = artifact.getLong("bytes");
            URL url = new URL(artifact.getString("url"));
            // ReleaseManifest binds the exact HTTPS origin and immutable path.
            connection = connections.open(url);
            HttpsURLConnection active = connection;
            deadline = new RequestDeadline(cancelled, active::disconnect, 120_000);
            deadline.checkActive();
            connection.setRequestMethod("GET"); connection.setInstanceFollowRedirects(false); connection.setUseCaches(false);
            connection.setConnectTimeout(15000); connection.setReadTimeout(15000);
            connection.setRequestProperty("Accept", "application/vnd.android.package-archive, application/octet-stream");
            connection.setRequestProperty("Accept-Encoding", "identity");
            if (connection.getResponseCode() != 200) throw new IllegalArgumentException();
            deadline.checkActive();
            long length = connection.getContentLengthLong();
            String encoding = connection.getHeaderField("Content-Encoding");
            if (length != -1 && length != expected || encoding != null && !encoding.equalsIgnoreCase("identity")) throw new IllegalArgumentException();
            File directory = context.getDir("release-stage", Context.MODE_PRIVATE);
            if (!android.system.OsConstants.S_ISDIR(android.system.Os.lstat(directory.getAbsolutePath()).st_mode)) throw new IllegalArgumentException();
            candidate = File.createTempFile("release-", ".apk", directory);
            try (InputStream input = connection.getInputStream(); FileOutputStream output = new FileOutputStream(candidate)) {
                byte[] buffer = new byte[65536]; long received = 0; int count;
                while ((count = input.read(buffer)) != -1) {
                    deadline.checkActive(); received += count;
                    if (received > expected) throw new IllegalArgumentException();
                    output.write(buffer,0,count);
                }
                if (received != expected) throw new IllegalArgumentException();
                output.getFD().sync();
            }
            deadline.checkActive();
            ReleaseArtifact.inspect(context.getPackageManager(), candidate, envelope, publicKey, origin, certificate,
                minimumSequence, context.getPackageName(), clock.get());
            deadline.checkActive(); completed = true;
            return candidate;
        } catch (Exception failure) { throw new IllegalArgumentException("Release download failed"); }
        finally {
            if (deadline != null) deadline.close();
            if (connection != null) try { connection.disconnect(); } catch (RuntimeException ignored) { }
            if (!completed && candidate != null && !candidate.delete()) throw new IllegalStateException("Release staging cleanup failed");
        }
    }
}
