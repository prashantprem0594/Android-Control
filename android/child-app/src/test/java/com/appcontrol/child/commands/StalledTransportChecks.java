package com.appcontrol.child.commands;

import java.io.IOException;
import java.io.InterruptedIOException;
import java.net.InetAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.net.URL;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLContext;

/** A real TLS client connects to a local peer that never completes handshake. */
public final class StalledTransportChecks {
    private static void require(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }

    private static void exercise(boolean cancel) throws Exception {
        AtomicBoolean stopped = new AtomicBoolean();
        CountDownLatch connected = new CountDownLatch(1);
        CountDownLatch releaseServer = new CountDownLatch(1);
        CountDownLatch disconnectAttempted = new CountDownLatch(1);
        ExecutorService workers = Executors.newFixedThreadPool(2, action -> {
            Thread thread = new Thread(action, "stalled-transport-fixture");
            thread.setDaemon(true);
            return thread;
        });
        try (ServerSocket listener = new ServerSocket(0, 1, InetAddress.getByName("127.0.0.1"))) {
            listener.setSoTimeout(3000);
            Future<?> peer = workers.submit(() -> {
                try (Socket socket = listener.accept()) {
                    connected.countDown();
                    // Read/write no certificate, response, credentials or payload.
                    releaseServer.await(8, TimeUnit.SECONDS);
                } catch (IOException | InterruptedException failure) {
                    throw new IllegalStateException("Silent TLS fixture failed");
                }
            });
            HttpsURLConnection connection = (HttpsURLConnection) new URL("https://127.0.0.1:" + listener.getLocalPort() + "/healthz").openConnection();
            connection.setConnectTimeout(1500);
            connection.setReadTimeout(1500);
            connection.setInstanceFollowRedirects(false);
            connection.setRequestMethod("GET");
            Future<Boolean> request = workers.submit(() -> {
                try (RequestControl control = new RequestControl(stopped::get, () -> {
                    disconnectAttempted.countDown();
                    connection.disconnect();
                }, cancel ? 10_000 : 500)) {
                    try {
                        control.checkActive();
                        connection.getResponseCode();
                        return false; // A silent TLS peer must never count as success.
                    } catch (IOException expected) {
                        try { control.checkActive(); return false; }
                        catch (InterruptedIOException cancelled) { return true; }
                    }
                } finally {
                    connection.disconnect();
                }
            });
            try {
                require(connected.await(3, TimeUnit.SECONDS), "TLS client never reached the stalled peer");
                if (cancel) stopped.set(true);
                require(disconnectAttempted.await(3, TimeUnit.SECONDS), "request did not attempt cancellation/disconnection");
                require(request.get(4, TimeUnit.SECONDS), "stalled TLS request did not fail closed");
            } finally {
                releaseServer.countDown();
                connection.disconnect();
            }
            peer.get(3, TimeUnit.SECONDS);
        } finally {
            releaseServer.countDown();
            workers.shutdownNow();
            require(workers.awaitTermination(3, TimeUnit.SECONDS), "transport fixture retained workers");
        }
    }

    public static void main(String[] args) throws Exception {
        SSLContext.getDefault(); // Warm platform trust initialization before timing.
        exercise(false);
        exercise(true);
        System.out.println("stalled-transport-checks-passed: real TLS handshake stall, deadline, cancellation, finite socket timeout and worker cleanup");
    }
}
