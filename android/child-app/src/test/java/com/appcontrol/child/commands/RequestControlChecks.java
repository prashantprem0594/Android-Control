package com.appcontrol.child.commands;

import java.io.InterruptedIOException;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

public final class RequestControlChecks {
    private static void require(boolean condition, String message) {
        if (!condition) throw new AssertionError(message);
    }

    public static void main(String[] args) throws Exception {
        AtomicBoolean stopped = new AtomicBoolean();
        AtomicInteger disconnects = new AtomicInteger();
        CountDownLatch disconnected = new CountDownLatch(1);
        try (RequestControl control = new RequestControl(stopped::get, () -> {
            disconnects.incrementAndGet(); disconnected.countDown();
        }, 10_000)) {
            control.checkActive();
            stopped.set(true);
            require(disconnected.await(2, TimeUnit.SECONDS), "cancelled request was not disconnected");
            for (int attempt = 0; attempt < 3; attempt++) {
                try { control.checkActive(); throw new AssertionError("cancelled request accepted work"); }
                catch (InterruptedIOException expected) { }
            }
            require(disconnects.get() == 1, "cancellation disconnected repeatedly");
        }
        CountDownLatch deadline = new CountDownLatch(1);
        try (RequestControl control = new RequestControl(() -> false, deadline::countDown, 25)) {
            require(deadline.await(2, TimeUnit.SECONDS), "request deadline did not disconnect");
            try { control.checkActive(); throw new AssertionError("expired request accepted work"); }
            catch (InterruptedIOException expected) { }
        }
        CountDownLatch afterClose = new CountDownLatch(1);
        RequestControl completed = new RequestControl(() -> false, afterClose::countDown, 100);
        completed.close();
        require(!afterClose.await(200, TimeUnit.MILLISECONDS), "closed request retained active watcher");
        try { completed.checkActive(); throw new AssertionError("closed request accepted work"); }
        catch (InterruptedIOException expected) { }
        try (RequestControl control = new RequestControl(() -> true, () -> { throw new IllegalStateException("Synthetic disconnect failure"); }, 1000)) {
            try { control.checkActive(); throw new AssertionError("disconnect failure bypassed cancellation"); }
            catch (InterruptedIOException expected) { }
        }
        for (long invalid : new long[]{0, -1, 300_001, Long.MAX_VALUE}) {
            try { new RequestControl(() -> false, () -> {}, invalid); throw new AssertionError("invalid request budget accepted"); }
            catch (IllegalArgumentException expected) { }
        }
        CountDownLatch signalFailure = new CountDownLatch(1);
        try (RequestControl control = new RequestControl(() -> { throw new IllegalStateException("fixture stop source failure"); }, signalFailure::countDown, 1000)) {
            try { control.checkActive(); throw new AssertionError("failed stop source accepted work"); }
            catch (InterruptedIOException expected) { }
            require(signalFailure.await(2, TimeUnit.SECONDS), "failed stop source did not disconnect");
        }
        System.out.println("request-control-checks-passed: cancellation, deadline, disconnect once, close cleanup, disconnect failure, invalid budgets");
    }
}
