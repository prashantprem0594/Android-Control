package com.appcontrol.child.testing;

import android.app.Service;
import android.content.Intent;
import android.content.SharedPreferences;
import android.os.Bundle;
import android.os.Handler;
import android.os.IBinder;
import android.os.Looper;
import android.os.Message;
import android.os.Messenger;
import android.os.Process;
import com.appcontrol.child.commands.CommandExecutor;
import java.time.Instant;

/** Debug-only separate process. Kills only itself, never performs a device action. */
public final class JournalProbeService extends Service {
    public static final int RESET = 1, EXECUTE = 2, KILL = 3;
    private Messenger messenger;

    @Override public void onCreate() {
        super.onCreate();
        messenger = new Messenger(new Handler(Looper.getMainLooper(), this::handle));
    }
    @Override public IBinder onBind(Intent intent) { return messenger.getBinder(); }

    private boolean handle(Message message) {
        SharedPreferences preferences = getSharedPreferences("fixture-process-journal", MODE_PRIVATE);
        Bundle reply = new Bundle();
        reply.putInt("pid", Process.myPid());
        try {
            if (message.what == KILL) {
                Process.killProcess(Process.myPid());
                return true;
            }
            if (message.what == RESET) {
                if (!preferences.edit().clear().commit()) throw new IllegalStateException();
            } else if (message.what == EXECUTE) {
                String id = message.getData().getString("id", "");
                String mode = message.getData().getString("mode", "normal");
                if (!id.matches("fixture-[a-z-]{1,40}") ||
                    !(mode.equals("normal") || mode.equals("after_reservation") || mode.equals("after_action"))) {
                    throw new IllegalArgumentException();
                }
                CommandExecutor executor = new CommandExecutor(new CommandExecutor.Journal() {
                    public String read(String command) { return preferences.getString(command, null); }
                    public boolean write(String command, String result, Instant deadline) {
                        boolean saved = preferences.edit().putString(command, result)
                            .putString(command + ".expiry", deadline.toString()).commit();
                        if (saved && result.equals("started") && mode.equals("after_reservation")) Process.killProcess(Process.myPid());
                        return saved;
                    }
                }, Instant::now);
                String result = executor.execute(id, Instant.now().plusSeconds(3600), () -> {
                    int count = preferences.getInt("actions." + id, 0) + 1;
                    if (!preferences.edit().putInt("actions." + id, count).commit()) throw new IllegalStateException();
                    if (mode.equals("after_action")) Process.killProcess(Process.myPid());
                });
                reply.putString("result", result);
                reply.putInt("actions", preferences.getInt("actions." + id, 0));
            } else throw new IllegalArgumentException();
        } catch (Exception failure) {
            reply.putBoolean("error", true);
        }
        if (message.replyTo != null) {
            try {
                Message response = Message.obtain(); response.setData(reply);
                message.replyTo.send(response);
            } catch (Exception ignored) { }
        }
        return true;
    }
}
