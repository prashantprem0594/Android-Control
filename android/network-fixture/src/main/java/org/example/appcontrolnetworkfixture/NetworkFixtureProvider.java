package org.example.appcontrolnetworkfixture;

import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.net.Uri;
import android.net.ConnectivityManager;
import android.net.NetworkCapabilities;
import android.os.Binder;
import android.os.Build;
import android.os.Bundle;
import android.os.Process;
import android.os.SystemClock;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.SocketTimeoutException;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.UUID;

/** One fixed raw-TCP endpoint under this test app's UID; never an arbitrary proxy. */
public final class NetworkFixtureProvider extends ContentProvider {
    @Override public boolean onCreate(){return true;}
    @Override public Bundle call(String method,String arg,Bundle extras){
        Context context=getContext();
        String[] callers=context.getPackageManager().getPackagesForUid(Binder.getCallingUid());
        if(!Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:") ||
            (context.getApplicationInfo().flags & ApplicationInfo.FLAG_DEBUGGABLE)==0 || callers==null ||
            !Arrays.asList(callers).contains("com.appcontrol.child") ||
            context.getPackageManager().checkSignatures(Binder.getCallingUid(),Process.myUid())!=PackageManager.SIGNATURE_MATCH){
            throw new SecurityException("Isolated same-signature child required");
        }
        if(!"probe".equals(method) || arg!=null || extras!=null && !extras.isEmpty())throw new IllegalArgumentException();
        long identity=Binder.clearCallingIdentity();
        try{
            Bundle result=new Bundle();String state;
            String nonce=UUID.randomUUID().toString().replace("-","");
            String expected="APP_CONTROL_NET_OK_V1:"+nonce+"\n";
            long deadline=SystemClock.elapsedRealtime()+3500;
            try(Socket socket=new Socket()){
                socket.connect(new InetSocketAddress("10.0.2.2",19089),1500);socket.setSoTimeout(1500);
                socket.getOutputStream().write(("APP_CONTROL_NET_FIXTURE_V1:"+nonce+"\n").getBytes(StandardCharsets.US_ASCII));
                StringBuilder response=new StringBuilder();
                while(response.length()<96 && SystemClock.elapsedRealtime()<deadline){
                    int value=socket.getInputStream().read();if(value<0)break;
                    response.append((char)value);if(value=='\n')break;
                }
                state=expected.contentEquals(response)?"reachable":"protocol_failed";
            }catch(SocketTimeoutException timeout){state="timeout";}
            catch(IOException failure){state=PacketFailure.status(failure);}
            result.putString("network_fixture",state);result.putString("network_fixture_dns",DnsWireFixture.status());
            result.putString("network_fixture_ipv6",NativeIpv6Fixture.status());
            result.putString("network_fixture_version","uid-tcp-dns-ipv6-host-v4");
            ConnectivityManager connectivity=context.getSystemService(ConnectivityManager.class);
            NetworkCapabilities capabilities=connectivity.getNetworkCapabilities(connectivity.getActiveNetwork());
            result.putBoolean("network_fixture_vpn",capabilities!=null && capabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN));
            boolean vpnAvailable=false;
            for(android.net.Network network:connectivity.getAllNetworks()){
                NetworkCapabilities candidate=connectivity.getNetworkCapabilities(network);
                if(candidate!=null && candidate.hasTransport(NetworkCapabilities.TRANSPORT_VPN)){vpnAvailable=true;break;}
            }
            result.putBoolean("network_fixture_vpn_available",vpnAvailable);
            return result;
        }finally{Binder.restoreCallingIdentity(identity);}
    }
    @Override public Cursor query(Uri uri,String[] projection,String selection,String[] args,String order){throw new UnsupportedOperationException();}
    @Override public String getType(Uri uri){return null;}
    @Override public Uri insert(Uri uri,ContentValues values){throw new UnsupportedOperationException();}
    @Override public int delete(Uri uri,String selection,String[] args){throw new UnsupportedOperationException();}
    @Override public int update(Uri uri,ContentValues values,String selection,String[] args){throw new UnsupportedOperationException();}
}
