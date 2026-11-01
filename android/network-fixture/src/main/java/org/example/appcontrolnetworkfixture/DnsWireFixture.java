package org.example.appcontrolnetworkfixture;

import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.InetAddress;
import java.net.SocketTimeoutException;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.UUID;

/** Fresh DNS-format UDP exchange at one fixed emulator gateway; no resolver/cache. */
public final class DnsWireFixture {
    private DnsWireFixture(){}
    public static String status(){
        String nonce=UUID.randomUUID().toString().replace("-","");
        try(DatagramSocket socket=new DatagramSocket()){
            ByteArrayOutputStream bytes=new ByteArrayOutputStream();
            DataOutputStream data=new DataOutputStream(bytes);
            data.writeShort(Integer.parseInt(nonce.substring(0,4),16));
            data.writeShort(0x0100);data.writeShort(1);data.writeShort(0);data.writeShort(0);data.writeShort(0);
            for(String label:new String[]{nonce,"fixture","invalid"}){
                byte[] encoded=label.getBytes(StandardCharsets.US_ASCII);data.writeByte(encoded.length);data.write(encoded);
            }
            data.writeByte(0);data.writeShort(1);data.writeShort(1);
            byte[] query=bytes.toByteArray();
            socket.connect(InetAddress.getByAddress(new byte[]{10,0,2,2}),19090);socket.setSoTimeout(1500);
            socket.send(new DatagramPacket(query,query.length));
            byte[] reply=new byte[513];DatagramPacket packet=new DatagramPacket(reply,reply.length);socket.receive(packet);
            byte[] expected=Arrays.copyOf(query,query.length+16);
            expected[2]=(byte)0x81;expected[3]=(byte)0x80;expected[7]=1;
            byte[] answer=new byte[]{(byte)0xc0,12,0,1,0,1,0,0,0,0,0,4,(byte)192,0,2,123};
            System.arraycopy(answer,0,expected,query.length,answer.length);
            return packet.getLength()==expected.length && Arrays.equals(Arrays.copyOf(reply,packet.getLength()),expected)?"reachable":"protocol_failed";
        }catch(SocketTimeoutException timeout){return "timeout";}
        catch(java.io.IOException failure){return PacketFailure.status(failure);}
        catch(Exception failure){return "failed";}
    }
}
