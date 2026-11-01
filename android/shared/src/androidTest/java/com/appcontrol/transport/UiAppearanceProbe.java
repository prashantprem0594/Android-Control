package com.appcontrol.transport;

import android.app.Activity;
import android.app.Instrumentation;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ApplicationInfo;
import android.graphics.Bitmap;
import android.graphics.Canvas;
import android.graphics.Rect;
import android.os.Build;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.View;
import android.view.WindowManager;
import java.io.File;
import java.io.FileOutputStream;
import java.util.ArrayList;
import java.util.List;

/** Synthetic emulator UI preview only. Never disables secure-window protection. */
public final class UiAppearanceProbe {
    private UiAppearanceProbe() {}

    private static void require(boolean condition) {
        if (!condition) throw new IllegalStateException();
    }

    private static void guard(Context context) {
        String target = context.getPackageName();
        require("com.appcontrol.parent.uipreview".equals(target)
                || "com.appcontrol.child.uipreview".equals(target));
        require((context.getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE) != 0);
        require(Build.FINGERPRINT.startsWith("google/sdk_gphone")
                && Build.FINGERPRINT.contains("/emu")
                && ("ranchu".equals(Build.HARDWARE) || "goldfish".equals(Build.HARDWARE)));
        require(context.getSharedPreferences("enrollment", Context.MODE_PRIVATE).getAll().isEmpty());
        require(context.getSharedPreferences("parent-session", Context.MODE_PRIVATE).getAll().isEmpty());
        require(context.getSharedPreferences("call-contact-cache", Context.MODE_PRIVATE).getAll().isEmpty());
        // A preview is only allowed in a completely empty acceptance preference
        // namespace, including histories and any future credential stores.
        File preferences = new File(context.getApplicationInfo().dataDir, "shared_prefs");
        File[] entries = preferences.listFiles();
        if (preferences.exists()) require(entries != null);
        if (entries != null) for (File entry : entries) {
            if (!entry.getName().endsWith(".xml")) continue;
            require(entry.isFile());
            String name = entry.getName().substring(0, entry.getName().length() - 4);
            require(context.getSharedPreferences(name, Context.MODE_PRIVATE).getAll().isEmpty());
        }
    }

    private static void preview(Context context, Instrumentation instrumentation,
            String className, String filename) {
        Activity activity = null;
        try {
            guard(context);
            Intent intent = new Intent().setClassName(context.getPackageName(), className)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK | Intent.FLAG_ACTIVITY_CLEAR_TASK);
            activity = instrumentation.startActivitySync(intent);
            require(activity != null);
            final Activity screen = activity;
            instrumentation.waitForIdleSync();
            final boolean[] laidOut = new boolean[1];
            long deadline = SystemClock.uptimeMillis() + 5000;
            do {
                instrumentation.runOnMainSync(() -> {
                    View decor = screen.getWindow().getDecorView();
                    View content = screen.findViewById(android.R.id.content);
                    laidOut[0] = decor.isLaidOut() && decor.getWidth() > 0 && decor.getHeight() > 0
                            && content != null && content.isLaidOut() && content.getWidth() > 0
                            && content.getHeight() > 0;
                });
                if (laidOut[0]) break;
                SystemClock.sleep(50);
            } while (SystemClock.uptimeMillis() < deadline);
            require(laidOut[0]);
            instrumentation.runOnMainSync(() -> {
                // Draw this synthetic app's own view tree into its private files.
                // FLAG_SECURE stays enabled; no display/screenshot API is used.
                screen.getWindow().addFlags(WindowManager.LayoutParams.FLAG_SECURE);
                require((screen.getWindow().getAttributes().flags
                        & WindowManager.LayoutParams.FLAG_SECURE) != 0);
                View decor = screen.getWindow().getDecorView();
                View content = screen.findViewById(android.R.id.content);
                Rect visible = new Rect();
                require(content.getGlobalVisibleRect(visible) && visible.width() > 0 && visible.height() > 0);
                require(content.getWidth() <= decor.getWidth() && content.getHeight() <= decor.getHeight());
                require(decor.getWidth() <= 4096 && decor.getHeight() <= 4096);
                Bitmap image = Bitmap.createBitmap(decor.getWidth(), decor.getHeight(), Bitmap.Config.ARGB_8888);
                try {
                    decor.draw(new Canvas(image));
                    try (FileOutputStream stream = context.openFileOutput(filename, Context.MODE_PRIVATE)) {
                        require(image.compress(Bitmap.CompressFormat.PNG, 100, stream));
                    } catch (java.io.IOException failure) { throw new IllegalStateException(); }
                } finally { image.recycle(); }
            });
            require(new File(context.getFilesDir(), filename).length() > 0);
        } finally {
            if (activity != null) {
                final Activity screen = activity;
                instrumentation.runOnMainSync(screen::finish);
                instrumentation.waitForIdleSync();
            }
        }
    }

    public static Bundle run(Context context, Instrumentation instrumentation) {
        Bundle result = new Bundle();
        List<String> paths = new ArrayList<>();
        String phase = "guard";
        try {
            guard(context);
            boolean child = "com.appcontrol.child.uipreview".equals(context.getPackageName());
            String namespace = child ? "com.appcontrol.child" : "com.appcontrol.parent";
            phase = "main";
            preview(context, instrumentation, namespace + (child ? ".MainActivity" : ".ParentActivity"), "ui-preview-main.png");
            paths.add(new File(context.getFilesDir(), "ui-preview-main.png").getAbsolutePath());
            if (child) {
                phase = "dial";
                preview(context, instrumentation, namespace + ".calls.DialActivity", "ui-preview-dial.png");
                paths.add(new File(context.getFilesDir(), "ui-preview-dial.png").getAbsolutePath());
                phase = "call";
                preview(context, instrumentation, namespace + ".calls.CallActivity", "ui-preview-call.png");
                paths.add(new File(context.getFilesDir(), "ui-preview-call.png").getAbsolutePath());
            }
            result.putString("ui_appearance_probe", "passed");
            result.putStringArrayList("ui_preview_paths", new ArrayList<>(paths));
            result.putString("ui_preview_scope", "empty-acceptance-emulator-only; no clicks or live-service verification");
        } catch (Exception | LinkageError failure) {
            result.putString("ui_appearance_probe", "failed");
            result.putString("ui_appearance_phase", phase);
        }
        return result;
    }
}
