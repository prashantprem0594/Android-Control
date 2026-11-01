package com.appcontrol.transport;

import java.io.ByteArrayInputStream;
import java.security.KeyStore;
import java.security.MessageDigest;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.util.Base64;
import javax.net.ssl.SSLContext;
import javax.net.ssl.TrustManager;
import javax.net.ssl.TrustManagerFactory;
import javax.net.ssl.X509TrustManager;

public final class AcceptanceHttps {
    private AcceptanceHttps() {}

    public static boolean isConfigured() {
        try {
            Class.forName("com.appcontrol.transport.AcceptanceTrustFixtureGenerated");
            return true;
        } catch (ClassNotFoundException missingFixture) {
            return false;
        }
    }

    public static javax.net.ssl.SSLSocketFactory socketFactory() throws Exception {
        Class<?> fixture = Class.forName("com.appcontrol.transport.AcceptanceTrustFixtureGenerated");
        byte[] rootDer = Base64.getDecoder().decode((String) fixture.getField("ROOT_CERTIFICATE_BASE64").get(null));
        X509Certificate root = (X509Certificate) CertificateFactory.getInstance("X.509")
            .generateCertificate(new ByteArrayInputStream(rootDer));
        String pin = (String) fixture.getField("LEAF_SPKI_PIN").get(null);
        KeyStore roots = KeyStore.getInstance(KeyStore.getDefaultType());
        roots.load(null);
        roots.setCertificateEntry("acceptance-root", root);
        TrustManagerFactory factory = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());
        factory.init(roots);
        X509TrustManager delegate = null;
        for (TrustManager manager : factory.getTrustManagers()) {
            if (manager instanceof X509TrustManager) {
                delegate = (X509TrustManager) manager;
                break;
            }
        }
        if (delegate == null) throw new IllegalStateException();
        X509TrustManager trustDelegate = delegate;
        X509TrustManager pinned = new X509TrustManager() {
            @Override public X509Certificate[] getAcceptedIssuers() { return trustDelegate.getAcceptedIssuers(); }
            @Override public void checkClientTrusted(X509Certificate[] chain, String authType)
                    throws java.security.cert.CertificateException {
                trustDelegate.checkClientTrusted(chain, authType);
            }
            @Override public void checkServerTrusted(X509Certificate[] chain, String authType)
                    throws java.security.cert.CertificateException {
                trustDelegate.checkServerTrusted(chain, authType);
                if (chain == null || chain.length == 0) throw new java.security.cert.CertificateException();
                try {
                    byte[] digest = MessageDigest.getInstance("SHA-256").digest(chain[0].getPublicKey().getEncoded());
                    String actual = "sha256/" + Base64.getEncoder().encodeToString(digest);
                    if (!actual.equals(pin)) throw new java.security.cert.CertificateException();
                } catch (java.security.cert.CertificateException failure) {
                    throw failure;
                } catch (Exception failure) {
                    throw new java.security.cert.CertificateException();
                }
            }
        };
        SSLContext ssl = SSLContext.getInstance("TLS");
        ssl.init(null, new TrustManager[]{pinned}, null);
        return ssl.getSocketFactory();
    }
}
