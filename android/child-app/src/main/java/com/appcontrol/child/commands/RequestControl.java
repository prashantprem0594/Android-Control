package com.appcontrol.child.commands;

import java.io.InterruptedIOException;
import java.util.function.BooleanSupplier;
import com.appcontrol.transport.RequestDeadline;

/** Per-request cancellation and elapsed-time budget; never inspects payloads. */
public final class RequestControl implements AutoCloseable {
    private final RequestDeadline deadline;

    public RequestControl(BooleanSupplier stopped, Runnable disconnect, long timeoutMillis) {
        deadline = new RequestDeadline(stopped,disconnect,timeoutMillis);
    }

    public void checkActive() throws InterruptedIOException {
        deadline.checkActive();
    }

    @Override public void close() {
        deadline.close();
    }
}
