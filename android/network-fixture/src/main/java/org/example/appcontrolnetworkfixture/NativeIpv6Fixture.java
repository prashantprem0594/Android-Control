package org.example.appcontrolnetworkfixture;

import android.os.SystemClock;
import java.net.Inet6Address;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.net.SocketTimeoutException;
import java.nio.charset.StandardCharsets;
import java.util.UUID;

/** Only native fec0::2, never loopback, a hostname or an IPv4-mapped address. */
public final class NativeIpv6Fixture {
    private NativeIpv6Fixture(){}
    public static String status(){
        String nonce=UUID.randomUUID().toString().replace("-","");
        String expected="APP_CONTROL_NET_OK_V1:"+nonce+"\n";
        try(Socket socket=new Socket()){
            byte[] raw=new byte[16];raw[0]=(byte)0xfe;raw[1]=(byte)0xc0;raw[15]=2;
            Inet6Address address=Inet6Address.getByAddress(null,raw,-1);
            if(address.isLoopbackAddress() || address.getAddress().length!=16)return "invalid_family";
            socket.connect(new InetSocketAddress(address,19091),1500);socket.setSoTimeout(1500);
            if(!(socket.getInetAddress() instanceof Inet6Address))return "invalid_family";
            socket.getOutputStream().write(("APP_CONTROL_NET_FIXTURE_V1:"+nonce+"\n").getBytes(StandardCharsets.US_ASCII));
            long deadline=SystemClock.elapsedRealtime()+2000;
            StringBuilder response=new StringBuilder();
            while(response.length()<96 && SystemClock.elapsedRealtime()<deadline){
                int value=socket.getInputStream().read();if(value<0)break;
                response.append((char)value);if(value=='\n')break;
            }
            return expected.contentEquals(response)?"reachable":"protocol_failed";
        }catch(SocketTimeoutException timeout){return "timeout";}
        catch(java.io.IOException failure){return PacketFailure.status(failure);}
        catch(Exception failure){return "failed";}
    }
}
