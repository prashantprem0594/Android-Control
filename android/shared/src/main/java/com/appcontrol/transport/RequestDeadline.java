package com.appcontrol.transport;

import java.io.InterruptedIOException;
import java.util.Objects;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.function.BooleanSupplier;

/** Per-request stop signal and monotonic elapsed-time budget for both apps. */
public final class RequestDeadline implements AutoCloseable {
    private final BooleanSupplier stopped;
    private final Runnable disconnect;
    private final long started = System.nanoTime();
    private final long budgetNanos;
    private final AtomicBoolean closed = new AtomicBoolean();
    private final AtomicBoolean cancelled = new AtomicBoolean();
    private final Thread watcher;
    public RequestDeadline(BooleanSupplier stopped, Runnable disconnect, long timeoutMillis) {
        if (timeoutMillis < 1 || timeoutMillis > 300000) throw new IllegalArgumentException("Invalid request budget");
        this.stopped = Objects.requireNonNull(stopped); this.disconnect = Objects.requireNonNull(disconnect);
        this.budgetNanos = timeoutMillis * 1000000L;
        watcher = new Thread(() -> {
            try { while (!closed.get()) { checkActive(); Thread.sleep(50); } }
            catch (InterruptedException | InterruptedIOException finished) { }
        }, "app-control-request-budget");
        watcher.setDaemon(true); watcher.start();
    }
    public void checkActive() throws InterruptedIOException {
        if (closed.get()) throw new InterruptedIOException("Request closed");
        boolean shouldStop;
        try { shouldStop = stopped.getAsBoolean(); } catch (RuntimeException failure) { shouldStop = true; }
        if (cancelled.get() || shouldStop || System.nanoTime() - started >= budgetNanos) {
            if (cancelled.compareAndSet(false, true)) { try { disconnect.run(); } catch (RuntimeException ignored) { } }
            throw new InterruptedIOException("Request stopped or timed out");
        }
    }
    @Override public void close() { closed.set(true); watcher.interrupt(); }
}
