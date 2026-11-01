package com.appcontrol.transport;

import android.app.Activity;
import android.content.Context;
import android.content.res.ColorStateList;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.graphics.drawable.RippleDrawable;
import android.graphics.drawable.StateListDrawable;
import android.os.Build;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.view.WindowInsets;
import android.view.WindowInsetsController;
import android.view.WindowManager;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.TextView;

/** Shared native styling for the parent and child apps. No behavior or policy changes. */
public final class AppAppearance {
    public static final int BACKGROUND = Color.rgb(245, 247, 251);
    public static final int INK = Color.rgb(23, 36, 59);
    public static final int BLUE = Color.rgb(49, 91, 219);
    public static final int MUTED = Color.rgb(101, 115, 139);
    private static final int BORDER = Color.rgb(218, 225, 237);
    private static final int DISABLED = Color.rgb(86, 100, 122);

    private AppAppearance() {}

    public static int dp(Context context, float value) {
        return Math.round(value * context.getResources().getDisplayMetrics().density);
    }

    @SuppressWarnings("deprecation")
    public static void prepare(Activity activity) {
        if (activity.getActionBar() != null) activity.getActionBar().hide();
        Window window = activity.getWindow();
        window.setBackgroundDrawableResource(android.R.color.white);
        window.getDecorView().setBackgroundColor(BACKGROUND);
        window.setStatusBarColor(BACKGROUND);
        window.setNavigationBarColor(BACKGROUND);
        window.getDecorView().setSystemUiVisibility(window.getDecorView().getSystemUiVisibility()
                | View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR | View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR);
        // Older releases already fit content around system bars. Keep that behavior
        // and let resize make focused fields reachable when the keyboard is shown.
        window.setSoftInputMode((window.getAttributes().softInputMode
                & ~WindowManager.LayoutParams.SOFT_INPUT_MASK_ADJUST)
                | WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);
        if (Build.VERSION.SDK_INT >= 29) window.setNavigationBarContrastEnforced(false);
        if (Build.VERSION.SDK_INT >= 30) {
            // Android 15+ enforces edge-to-edge at our target SDK. Use one inset
            // owner on all API30+ releases; never also pad a child or the decor.
            window.setDecorFitsSystemWindows(false);
            WindowInsetsController controller = window.getInsetsController();
            if (controller != null) {
                int lightBars = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS
                        | WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS;
                controller.setSystemBarsAppearance(lightBars, lightBars);
            }
            View content = activity.findViewById(android.R.id.content);
            if (content != null) {
                final int left = content.getPaddingLeft(), top = content.getPaddingTop();
                final int right = content.getPaddingRight(), bottom = content.getPaddingBottom();
                content.setFitsSystemWindows(false);
                content.setOnApplyWindowInsetsListener((view, insets) -> {
                    // getInsets takes the maximum per edge: IME and navigation-bar
                    // heights must not be added together. Cutouts also matter in landscape.
                    android.graphics.Insets safe = insets.getInsets(WindowInsets.Type.systemBars()
                            | WindowInsets.Type.displayCutout() | WindowInsets.Type.ime());
                    view.setPadding(left + safe.left, top + safe.top,
                            right + safe.right, bottom + safe.bottom);
                    return WindowInsets.CONSUMED;
                });
                content.post(content::requestApplyInsets);
            }
        }
    }

    public static LinearLayout column(Context context) {
        LinearLayout layout = new LinearLayout(context);
        layout.setOrientation(LinearLayout.VERTICAL);
        layout.setBackgroundColor(BACKGROUND);
        layout.setPadding(dp(context, 20), dp(context, 24), dp(context, 20), dp(context, 32));
        layout.setLayoutParams(new ViewGroup.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));
        return layout;
    }

    public static LinearLayout card(Context context) {
        LinearLayout layout = new LinearLayout(context);
        layout.setOrientation(LinearLayout.VERTICAL);
        layout.setPadding(dp(context, 18), dp(context, 18), dp(context, 18), dp(context, 18));
        layout.setBackground(shape(context, Color.WHITE, 20, BORDER, 1));
        LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        params.setMargins(0, 0, 0, dp(context, 16));
        layout.setLayoutParams(params);
        return layout;
    }

    public static TextView text(Context context, String value, float size, boolean bold) {
        TextView view = new TextView(context);
        view.setText(value);
        view.setTextSize(size);
        view.setTextColor(INK);
        view.setTypeface(Typeface.create("sans-serif", bold ? Typeface.BOLD : Typeface.NORMAL));
        view.setLineSpacing(dp(context, 3), 1f);
        view.setPadding(0, 0, 0, dp(context, 10));
        return view;
    }

    public static void styleButton(Button button, boolean primary) {
        Context context = button.getContext();
        button.setAllCaps(false);
        button.setTextSize(16);
        button.setTypeface(Typeface.create("sans-serif-medium", Typeface.NORMAL));
        button.setGravity(Gravity.CENTER);
        button.setMinHeight(dp(context, 52));
        button.setMinimumHeight(dp(context, 52));
        button.setPadding(dp(context, 18), dp(context, 12), dp(context, 18), dp(context, 12));
        button.setTextColor(new ColorStateList(new int[][] {
                new int[] {-android.R.attr.state_enabled}, new int[] {}
        }, new int[] {DISABLED, primary ? Color.WHITE : BLUE}));
        StateListDrawable surfaces = new StateListDrawable();
        surfaces.addState(new int[] {-android.R.attr.state_enabled},
                shape(context, Color.rgb(229, 234, 243), 14, BORDER, 1));
        surfaces.addState(new int[] {android.R.attr.state_focused},
                shape(context, primary ? BLUE : Color.WHITE, 14, INK, 2));
        surfaces.addState(new int[] {},
                shape(context, primary ? BLUE : Color.WHITE, 14, primary ? BLUE : BORDER, 1));
        button.setBackgroundTintList(null);
        button.setBackground(new RippleDrawable(ColorStateList.valueOf(primary
                ? Color.argb(55, 255, 255, 255) : Color.argb(32, 49, 91, 219)), surfaces, null));
        button.setElevation(0);
        button.setStateListAnimator(null);
        if (button.getLayoutParams() == null) {
            LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            params.setMargins(0, dp(context, 5), 0, dp(context, 5));
            button.setLayoutParams(params);
        }
    }

    public static void styleField(EditText field) {
        Context context = field.getContext();
        field.setTextSize(16);
        field.setTextColor(new ColorStateList(new int[][] {
                new int[] {-android.R.attr.state_enabled}, new int[] {}
        }, new int[] {DISABLED, INK}));
        field.setHintTextColor(MUTED);
        field.setMinHeight(dp(context, 54));
        field.setPadding(dp(context, 14), dp(context, 12), dp(context, 14), dp(context, 12));
        StateListDrawable surfaces = new StateListDrawable();
        surfaces.addState(new int[] {-android.R.attr.state_enabled},
                shape(context, Color.rgb(237, 240, 246), 12, BORDER, 1));
        surfaces.addState(new int[] {android.R.attr.state_focused},
                shape(context, Color.WHITE, 12, BLUE, 2));
        surfaces.addState(new int[] {}, shape(context, Color.WHITE, 12, BORDER, 1));
        field.setBackgroundTintList(null);
        field.setBackground(surfaces);
        if (field.getLayoutParams() == null) {
            LinearLayout.LayoutParams params = new LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
            params.setMargins(0, dp(context, 5), 0, dp(context, 9));
            field.setLayoutParams(params);
        }
    }

    public static void status(TextView view) {
        Context context = view.getContext();
        view.setTextColor(MUTED);
        view.setTextSize(14);
        view.setLineSpacing(dp(context, 3), 1f);
        view.setPadding(0, dp(context, 8), 0, dp(context, 12));
        view.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
    }

    private static GradientDrawable shape(Context context, int fill, float radius,
            int stroke, float strokeWidth) {
        GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(fill);
        drawable.setCornerRadius(dp(context, radius));
        drawable.setStroke(dp(context, strokeWidth), stroke);
        return drawable;
    }
}
