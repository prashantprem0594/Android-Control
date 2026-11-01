package com.appcontrol.transport;

import android.app.Activity;
import android.content.Context;
import android.content.pm.PackageManager;
import android.net.InetAddresses;
import android.os.Build;
import java.net.InetAddress;
import java.net.URI;
import java.util.Locale;

/** Android 17 LAN consent. No discovery, DNS lookup, trust override or background prompt. */
public final class LocalNetworkAccess {
    public static final String PERMISSION = "android.permission.ACCESS_LOCAL_NETWORK";
    private LocalNetworkAccess() {}
    public static boolean supported(Context context) {
        return Build.VERSION.SDK_INT >= 37 && context.getApplicationInfo().targetSdkVersion >= 37;
    }
    public static boolean granted(Context context) {
        return !supported(context) || context.checkSelfPermission(PERMISSION) == PackageManager.PERMISSION_GRANTED;
    }
    public static void request(Activity activity, int requestCode) {
        if (!granted(activity)) activity.requestPermissions(new String[]{PERMISSION}, requestCode);
    }
    /** Recognizes only obvious local origins; private DNS names use the explicit setup button. */
    public static boolean neededForOrigin(Context context, String origin) {
        if (Build.VERSION.SDK_INT < 37) return false;
        if (granted(context)) return false;
        try {
            URI parsed = new URI(origin.trim());
            if (!"https".equalsIgnoreCase(parsed.getScheme()) || parsed.getRawUserInfo() != null) return false;
            String host = parsed.getHost();
            if (host == null) return false;
            host = host.toLowerCase(Locale.ROOT);
            if (host.equals("localhost")) return false;
            if (host.endsWith(".local") || host.endsWith(".home.arpa")) return true;
            if (host.startsWith("[") && host.endsWith("]")) host = host.substring(1, host.length() - 1);
            if (!InetAddresses.isNumericAddress(host)) return false;
            InetAddress address = InetAddresses.parseNumericAddress(host);
            if (address.isLoopbackAddress()) return false;
            byte[] bytes = address.getAddress();
            return address.isSiteLocalAddress() || address.isLinkLocalAddress() || address.isMulticastAddress() ||
                bytes.length == 16 && (bytes[0] & 0xfe) == 0xfc;
        } catch (Exception invalid) { return false; }
    }
}
