package com.appcontrol.transport;

import android.content.Context;
import android.content.SharedPreferences;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Instant;
import java.util.Arrays;
import java.util.Base64;
import java.util.HashSet;
import org.json.JSONObject;

/** Persistent replay floor scoped to independently provisioned update authority.
 * Accept before downloading. A failed download may retry the same signed payload;
 * it cannot roll back to an older sequence or different content at that sequence.
 */
public final class ReleaseState {
    private static final Object LOCK = new Object();
    private final SharedPreferences state;
    private final String publicKey, origin, certificate, binding;
    private final long compiledMinimum;
    public ReleaseState(Context context, String publicKey, String origin, String certificate, long compiledMinimum) {
        this(context, publicKey, origin, certificate, compiledMinimum, "release-state");
    }
    ReleaseState(Context context, String publicKey, String origin, String certificate, long compiledMinimum, String name) {
        if (compiledMinimum < 0 || compiledMinimum > 2100000000L || !name.matches("[a-z-]{1,64}")) throw new IllegalArgumentException("Invalid update state configuration");
        this.publicKey=publicKey;this.origin=origin;this.certificate=certificate;this.compiledMinimum=compiledMinimum;
        this.binding=digest((publicKey+"\n"+origin+"\n"+certificate).getBytes(StandardCharsets.UTF_8));
        this.state=context.getApplicationContext().getSharedPreferences(name,Context.MODE_PRIVATE);
    }
    private static String digest(byte[] bytes) {
        try {
            byte[] hash=MessageDigest.getInstance("SHA-256").digest(bytes);StringBuilder text=new StringBuilder();
            for(byte value:hash)text.append(String.format(java.util.Locale.ROOT,"%02x",value&255));
            return text.toString();
        } catch(Exception failure){throw new IllegalStateException("Update state unavailable");}
    }
    private long storedSequence() {
        try {
            if(state.getAll().isEmpty())return 0;
            if(!state.getAll().keySet().equals(new HashSet<>(Arrays.asList("version","binding","sequence","payloadSHA256"))) ||
                state.getInt("version",0)!=1 || !binding.equals(state.getString("binding",null)) ||
                !state.getString("payloadSHA256","").matches("[a-f0-9]{64}"))throw new IllegalStateException();
            long sequence=state.getLong("sequence",0);
            if(sequence<1||sequence>2100000000L)throw new IllegalStateException();
            return sequence;
        }catch(Exception failure){throw new IllegalStateException("Update state invalid");}
    }
    public long minimumSequence() {
        synchronized(LOCK){return Math.max(compiledMinimum,storedSequence());}
    }
    public JSONObject verifyAndRemember(byte[] envelope, Instant now) {
        synchronized(LOCK){
            try {
                byte[] snapshot=envelope.clone();
                long previous=storedSequence();
                JSONObject manifest=ReleaseManifest.verify(snapshot,publicKey,origin,certificate,Math.max(compiledMinimum,previous),now);
                long sequence=manifest.getLong("sequence");
                JSONObject outer=new JSONObject(new String(snapshot,StandardCharsets.UTF_8));
                String payloadHash=digest(Base64.getUrlDecoder().decode(outer.getString("payload")));
                if(sequence==previous){
                    if(!payloadHash.equals(state.getString("payloadSHA256",null)))throw new IllegalStateException();
                }else if(!state.edit().clear().putInt("version",1).putString("binding",binding).putLong("sequence",sequence).putString("payloadSHA256",payloadHash).commit())throw new IllegalStateException();
                return manifest;
            }catch(Exception failure){throw new IllegalArgumentException("Release replay protection failed");}
        }
    }
}
