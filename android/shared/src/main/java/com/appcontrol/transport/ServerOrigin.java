package com.appcontrol.transport;

import java.net.URI;
import java.net.URL;
import java.util.Locale;

/** Origin gate before opening a connection or accessing enrollment credentials. */
public final class ServerOrigin {
    private ServerOrigin() {}

    public static URL require(String supplied, String configured) {
        try {
            String canonical = canonical(supplied);
            if (!configured.isEmpty() && !canonical.equals(canonical(configured))) {
                throw new IllegalArgumentException("Server differs from configured origin");
            }
            return new URI(canonical).toURL();
        } catch (IllegalArgumentException e) {
            throw e;
        } catch (Exception e) {
            throw new IllegalArgumentException("An HTTPS server origin is required");
        }
    }

    private static String canonical(String supplied) throws Exception {
        if (supplied == null || supplied.length() > 2048) throw new IllegalArgumentException("Invalid server origin");
        URI uri = new URI(supplied.trim());
        String host = uri.getHost();
        if (!"https".equals(uri.getScheme()) || host == null || host.isEmpty() ||
                uri.getRawUserInfo() != null || uri.getRawQuery() != null || uri.getRawFragment() != null ||
                !(uri.getRawPath() == null || uri.getRawPath().isEmpty() || "/".equals(uri.getRawPath())) ||
                (uri.getPort() != -1 && (uri.getPort() < 1 || uri.getPort() > 65535)) || host.endsWith(".")) {
            throw new IllegalArgumentException("An HTTPS server origin is required");
        }
        return new URI("https", null, host.toLowerCase(Locale.ROOT), uri.getPort() == 443 ? -1 : uri.getPort(), null, null, null).toASCIIString();
    }
}
