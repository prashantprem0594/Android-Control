package com.appcontrol.child;

import android.os.Bundle;
import android.util.Base64;
import com.appcontrol.child.enrollment.DeviceIdentity;
import java.nio.charset.StandardCharsets;
import java.security.KeyFactory;
import java.security.KeyStore;
import java.security.MessageDigest;
import java.security.PublicKey;
import java.security.SecureRandom;
import java.security.Signature;
import java.security.interfaces.ECPublicKey;
import java.security.spec.X509EncodedKeySpec;

/** Exercises production AndroidKeyStore identity without network or enrollment. */
public final class DeviceIdentityProbe {
    private DeviceIdentityProbe() {}

    private static byte[] decode(String value) {
        return Base64.decode(value, Base64.URL_SAFE | Base64.NO_WRAP | Base64.NO_PADDING);
    }
    private static boolean verify(PublicKey key, String prefix, byte[] challenge, String proof) throws Exception {
        Signature verifier = Signature.getInstance("SHA256withECDSA");
        verifier.initVerify(key);
        verifier.update(prefix.getBytes(StandardCharsets.UTF_8));
        verifier.update(challenge);
        return verifier.verify(decode(proof));
    }
    private static void require(boolean condition) {
        if (!condition) throw new IllegalStateException("Identity invariant failed");
    }
    public static Bundle run() {
        Bundle result = new Bundle();
        boolean passed = false;
        try {
            DeviceIdentity identity = new DeviceIdentity();
            String encoded = identity.publicKey();
            require(encoded.equals(new DeviceIdentity().publicKey()));
            PublicKey publicKey = KeyFactory.getInstance("EC").generatePublic(new X509EncodedKeySpec(decode(encoded)));
            require(publicKey instanceof ECPublicKey && ((ECPublicKey) publicKey).getParams().getOrder().bitLength() == 256);
            KeyStore store = KeyStore.getInstance("AndroidKeyStore");
            store.load(null);
            require(store.getKey("app-control-device-v1", null).getEncoded() == null);
            byte[] challenge = new byte[32];
            new SecureRandom().nextBytes(challenge);
            String value = Base64.encodeToString(challenge, Base64.URL_SAFE | Base64.NO_WRAP | Base64.NO_PADDING);
            String enrollment = "enr_AAAAAAAAAAAAAAAAAAAAAA";
            String device = "dev_BBBBBBBBBBBBBBBBBBBBBB";
            String auth = "dac_CCCCCCCCCCCCCCCCCCCCCC";
            String proof = identity.sign(enrollment, value);
            String enrollmentPrefix = "app-control-device-enrollment-v1\n" + enrollment + "\n";
            require(verify(publicKey, enrollmentPrefix, challenge, proof));
            byte[] changed = challenge.clone(); changed[0] ^= 1;
            require(!verify(publicKey, enrollmentPrefix, changed, proof));
            String authPrefix = "app-control-device-auth-v1\n" + device + "\n" + auth + "\n";
            require(!verify(publicKey, authPrefix, challenge, proof));
            require(verify(publicKey, authPrefix, challenge, identity.signAuthentication(device, auth, value)));
<<<<<<< HEAD
            String recoveryRequest = "drr_DDDDDDDDDDDDDDDDDDDDDD";
            String recoveryPrefix = "app-control-device-recovery-v1\n" + recoveryRequest + "\n";
            String recoveryProof = identity.signRecovery(recoveryRequest, value);
            require(verify(publicKey, recoveryPrefix, challenge, recoveryProof));
            require(!verify(publicKey, enrollmentPrefix, challenge, recoveryProof));
            require(!verify(publicKey, recoveryPrefix, changed, recoveryProof));
=======
>>>>>>> origin/main
            StringBuilder fingerprint = new StringBuilder();
            for (byte part : MessageDigest.getInstance("SHA-256").digest(decode(encoded))) fingerprint.append(String.format("%02x", part & 255));
            result.putString("public_key_sha256", fingerprint.toString());
            passed = true;
        } catch (Exception failure) {
            // Never print challenge, proof, key objects, supplied values or errors.
        }
        result.putString("identity_probe", passed ? "passed" : "failed");
        result.putString("stream", "Android Keystore identity probe: " + (passed ? "passed" : "failed") + "\n");
        return result;
    }
}
