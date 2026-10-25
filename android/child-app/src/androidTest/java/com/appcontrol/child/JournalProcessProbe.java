package com.appcontrol.child;

import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.ServiceConnection;
import android.os.Bundle;
import android.os.Handler;
import android.os.HandlerThread;
import android.os.IBinder;
import android.os.Message;
import android.os.Messenger;
import com.appcontrol.child.testing.JournalProbeService;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;

/** Real remote-process death with the production executor and Android preferences. */
public final class JournalProcessProbe {
    private static void require(boolean condition) { if (!condition) throw new IllegalStateException("Journal invariant failed"); }
    private static final class Client implements AutoCloseable {
        final Context context;
        final HandlerThread replies = new HandlerThread("journal-probe-replies");
        final BlockingQueue<Bundle> responses = new LinkedBlockingQueue<>();
        final CountDownLatch connected = new CountDownLatch(1);
        final CountDownLatch died = new CountDownLatch(1);
        volatile Messenger remote;
        Messenger receiver;
        boolean bound;
        final ServiceConnection connection = new ServiceConnection() {
            public void onServiceConnected(ComponentName name, IBinder binder) {
                try { binder.linkToDeath(died::countDown, 0); }
                catch (Exception failure) { died.countDown(); }
                remote = new Messenger(binder); connected.countDown();
            }
            public void onServiceDisconnected(ComponentName name) { died.countDown(); }
        };
        Client(Context context) throws Exception {
            this.context = context;
            replies.start();
            receiver = new Messenger(new Handler(replies.getLooper(), message -> { responses.add(new Bundle(message.getData())); return true; }));
            try {
                bound = context.bindService(new Intent(context, JournalProbeService.class), connection, Context.BIND_AUTO_CREATE);
                require(bound && connected.await(15, TimeUnit.SECONDS));
            } catch (Exception failure) { close(); throw failure; }
        }
        void send(int operation, String id, String mode) throws Exception {
            Message message = Message.obtain(null, operation);
            message.replyTo = receiver;
            message.getData().putString("id", id);
            message.getData().putString("mode", mode);
            remote.send(message);
        }
        Bundle call(int operation, String id, String mode) throws Exception {
            send(operation, id, mode);
            Bundle result = responses.poll(15, TimeUnit.SECONDS);
            require(result != null && !result.getBoolean("error"));
            return result;
        }
        void crash(int operation, String id, String mode) throws Exception {
            send(operation, id, mode);
            require(died.await(15, TimeUnit.SECONDS));
        }
        public void close() {
            if (bound) { context.unbindService(connection); bound = false; }
            replies.quitSafely();
            try { replies.join(2000); } catch (InterruptedException failure) { Thread.currentThread().interrupt(); }
        }
    }
    private static void verify(Bundle result, String outcome, int actions, int previousPid) {
        require(outcome.equals(result.getString("result")) && result.getInt("actions") == actions);
        require(result.getInt("pid") != previousPid);
    }
    public static Bundle run(Context context) {
        Bundle result = new Bundle();
        String stage = "successful_result";
        try {
            int pid;
            try (Client first = new Client(context)) {
                first.call(JournalProbeService.RESET, "", "normal");
                Bundle completed = first.call(JournalProbeService.EXECUTE, "fixture-success", "normal");
                verify(completed, "succeeded", 1, android.os.Process.myPid());
                pid = completed.getInt("pid");
                first.crash(JournalProbeService.KILL, "", "normal");
            }
            stage = "lost_ack_restart";
            try (Client second = new Client(context)) {
                Bundle retried = second.call(JournalProbeService.EXECUTE, "fixture-success", "normal");
                verify(retried, "succeeded", 1, pid);
                pid = retried.getInt("pid");
                stage = "reservation_crash";
                second.crash(JournalProbeService.EXECUTE, "fixture-reservation", "after_reservation");
            }
            stage = "reservation_restart";
            try (Client third = new Client(context)) {
                Bundle reserved = third.call(JournalProbeService.EXECUTE, "fixture-reservation", "normal");
                verify(reserved, "failed", 0, pid);
                pid = reserved.getInt("pid");
                stage = "action_crash";
                third.crash(JournalProbeService.EXECUTE, "fixture-action", "after_action");
            }
            stage = "uncertain_action_restart";
            try (Client fourth = new Client(context)) {
                Bundle uncertain = fourth.call(JournalProbeService.EXECUTE, "fixture-action", "normal");
                verify(uncertain, "failed", 1, pid);
                fourth.call(JournalProbeService.RESET, "", "normal");
                fourth.crash(JournalProbeService.KILL, "", "normal");
            }
            result.putString("journal_probe", "passed");
            result.putString("stream", "Android journal probe: process death, durable reservation/result and no repeated action passed\n");
        } catch (Exception failure) {
            result.putString("journal_probe", "failed");
            result.putString("observation", stage);
            result.putString("stream", "Android journal probe failed at " + stage + "\n");
        }
        return result;
    }
}
