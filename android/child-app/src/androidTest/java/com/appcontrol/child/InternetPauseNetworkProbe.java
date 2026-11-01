package com.appcontrol.child;

import android.app.Instrumentation;
import android.app.admin.DevicePolicyManager;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ApplicationInfo;
import android.content.pm.PackageManager;
import android.net.ConnectivityManager;
import android.net.NetworkCapabilities;
import android.net.Uri;
import android.net.VpnService;
import android.os.Build;
import android.os.Bundle;
import android.os.SystemClock;
import android.view.accessibility.AccessibilityNodeInfo;
import com.appcontrol.child.admin.AppControlDeviceAdminReceiver;
import com.appcontrol.child.policy.InternetPausePlan;
import com.appcontrol.child.policy.InternetPauseVpn;
import com.appcontrol.child.policy.PolicyEnforcer;
import com.appcontrol.transport.LocalNetworkAccess;
import java.io.ByteArrayOutputStream;
import java.io.DataOutputStream;
import java.net.DatagramPacket;
import java.net.DatagramSocket;
import java.net.InetAddress;
import java.net.Inet6Address;
import java.net.SocketTimeoutException;
import java.nio.charset.StandardCharsets;
import java.util.Arrays;
import java.util.UUID;
import java.io.IOException;
import java.net.Socket;
import java.net.InetSocketAddress;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import org.json.JSONArray;
import org.json.JSONObject;

/** IPv4 TCP/DNS-wire and baseline-gated native IPv6 checks on the isolated test AVD. */
public final class InternetPauseNetworkProbe {
    private static final class DnsWireFixture {
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


    private static final class NativeIpv6Fixture {
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

    private static final class PacketFailure {
        private PacketFailure(){}
        static String status(java.io.IOException failure){
            Throwable cause=failure;
            for(int depth=0;cause!=null && depth<6;depth++,cause=cause.getCause()){
                if(cause instanceof java.net.NoRouteToHostException)return "no_route";
                if(cause instanceof android.system.ErrnoException){
                    int errno=((android.system.ErrnoException)cause).errno;
                    if(errno==android.system.OsConstants.ENETUNREACH)return "network_unreachable";
                    if(errno==android.system.OsConstants.EHOSTUNREACH)return "host_unreachable";
                }
            }
            return "unreachable";
        }
    }

    private static final String TARGET="org.example.appcontrolnetworkfixture";
    private static final String ACCEPTANCE_CHILD="com.appcontrol.child.acceptance";
    private static final String JOURNAL="fixture-network-pause";
    private static void require(boolean value){if(!value)throw new IllegalStateException();}
    // A disappeared route is a valid packet block only after a reachable
    // baseline; every accepted run also verifies management reachability and
    // target restoration. Refusal, protocol and arbitrary IO failures fail.
    private static boolean packetBlocked(String state){
        return "timeout".equals(state) || "no_route".equals(state) ||
            "network_unreachable".equals(state) || "host_unreachable".equals(state);
    }
    private static SharedPreferences cache(Context context){return context.getSharedPreferences("policy-cache",Context.MODE_PRIVATE);}
    private static String vpnSummary(Context context){
        ConnectivityManager connectivity=context.getSystemService(ConnectivityManager.class);
        StringBuilder summary=new StringBuilder();
        for(android.net.Network network:connectivity.getAllNetworks()){
            NetworkCapabilities capabilities=connectivity.getNetworkCapabilities(network);
            if(capabilities!=null && capabilities.hasTransport(NetworkCapabilities.TRANSPORT_VPN)){
                if(summary.length()>0)summary.append(';');
                summary.append(network).append(' ').append(capabilities);
            }
        }
        return summary.length()==0?"none":summary.toString();
    }
    private static String managementStatus(){
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
            return expected.contentEquals(response)?"reachable":"protocol_failed";
        }catch(SocketTimeoutException timeout){return "timeout";}
        catch(IOException failure){return PacketFailure.status(failure);}
        catch(Exception failure){return "failed";}
    }
    private static boolean managementReachable(){return "reachable".equals(managementStatus());}
    private static String tunnelState(){
        try{
            java.lang.reflect.Field field=InternetPauseVpn.class.getDeclaredField("active");field.setAccessible(true);
            Object session=field.get(null);if(session==null)return "absent";
            java.lang.reflect.Field descriptor=session.getClass().getDeclaredField("descriptor");descriptor.setAccessible(true);
            android.os.ParcelFileDescriptor value=(android.os.ParcelFileDescriptor)descriptor.get(session);
            return value.getFileDescriptor().valid()?"descriptor_valid":"descriptor_invalid";
        }catch(Exception failure){return "unavailable";}
    }
    private static Bundle fixture(Context context){
        Bundle reply=context.getContentResolver().call(Uri.parse("content://"+TARGET+".control"),"probe",null,null);
        require(reply!=null && "uid-tcp-dns-ipv6-host-v4".equals(reply.getString("network_fixture_version")));
        return reply;
    }
    private static void policy(Context context,long version,boolean paused,JSONArray allowed) throws Exception{
        JSONObject desired=new JSONObject().put("formatVersion",2).put("version",version).put("internetPaused",paused)
            .put("blockedPackages",new JSONArray()).put("alwaysAllowedPackages",allowed).put("schedules",new JSONArray());
        require(cache(context).edit().putLong("version",version).putString("desired",desired.toString()).commit());
    }
    private static String baselineFailure(Context context){
        SharedPreferences stored=cache(context), ledger=context.getSharedPreferences("package-reconciliation",Context.MODE_PRIVATE);
        if(stored.getString("desired",null)!=null)return "baseline_policy_cache_not_empty";
        if(!Collections.singleton("alarmMode").containsAll(stored.getAll().keySet()))return "baseline_policy_cache_keys";
        if(!Collections.singleton("tracked").containsAll(ledger.getAll().keySet()))return "baseline_reconciliation_keys";
        if(!ledger.getStringSet("tracked",Collections.emptySet()).isEmpty())return "baseline_reconciliation_not_empty";
        if(!"none".equals(vpnSummary(context)))return "baseline_vpn_present";
        for(android.content.pm.PackageInfo info:context.getPackageManager().getInstalledPackages(PackageManager.GET_SERVICES)){
            // The HTTPS acceptance app is deliberately side-by-side and may use a different debug signer.
            // It is only exempted as a declared service when its exact package is still debuggable;
            // the active-network VPN snapshot above must independently remain empty.
            boolean acceptanceSibling=ACCEPTANCE_CHILD.equals(info.packageName) && info.applicationInfo!=null &&
                (info.applicationInfo.flags & ApplicationInfo.FLAG_DEBUGGABLE)!=0;
            if(info.applicationInfo==null || (info.applicationInfo.flags & ApplicationInfo.FLAG_SYSTEM)!=0 || context.getPackageName().equals(info.packageName) || acceptanceSibling)continue;
            if(info.services!=null)for(android.content.pm.ServiceInfo service:info.services)if("android.permission.BIND_VPN_SERVICE".equals(service.permission))return "baseline_other_user_vpn_service:"+info.packageName;
        }
        return null;
    }
    private static void cleanup(Context context) throws Exception{
        SharedPreferences journal=context.getSharedPreferences(JOURNAL,Context.MODE_PRIVATE);
        if(journal.getAll().isEmpty())return;
        require(journal.getBoolean("pending",false));
        String raw=cache(context).getString("desired",null);
        if(raw!=null){
            JSONObject desired=new JSONObject(raw);
            require(desired.getLong("version")>=701 && desired.getLong("version")<=703);
            require(desired.getJSONArray("blockedPackages").length()==0 && desired.getJSONArray("schedules").length()==0);
        }
        require(InternetPauseVpn.Companion.reconcile(context,703,false));
        if(raw!=null){
            policy(context,703,false,new JSONArray(journal.getString("allowed","[]")));
            PackagePolicyProbe.applied(new PolicyEnforcer(context).applyCached());
        }
        Map<String,Object> oldCache=new HashMap<>(), oldLedger=new HashMap<>();
        if(journal.getBoolean("hadAlarmMode",false))oldCache.put("alarmMode",journal.getString("alarmMode","none"));
        if(journal.getBoolean("hadTracked",false))oldLedger.put("tracked",Collections.emptySet());
        PackagePolicyProbe.restore(context.getSharedPreferences("package-reconciliation",Context.MODE_PRIVATE),oldLedger);
        PackagePolicyProbe.restore(cache(context),oldCache);
        require(InternetPauseVpn.Companion.reconcile(context,703,false));
        require(context.deleteSharedPreferences(JOURNAL));
    }
    private static void consent(Context context,Instrumentation instrumentation) throws Exception{
        if(!LocalNetworkAccess.granted(context)){
            TelecomFixtureProbe.main(instrumentation,()->{context.startActivity(new Intent(context,MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));return null;});
            TelecomFixtureProbe.control(instrumentation,"Allow home-network server access",true);
            long localDeadline=SystemClock.elapsedRealtime()+25000;
            while(!LocalNetworkAccess.granted(context) && SystemClock.elapsedRealtime()<localDeadline){
                AccessibilityNodeInfo root=instrumentation.getUiAutomation().getRootInActiveWindow();
                if(root!=null)for(AccessibilityNodeInfo node:root.findAccessibilityNodeInfosByText("Allow")){
                    String owner=node.getPackageName()==null?"":node.getPackageName().toString();
                    if((owner.equals("com.android.permissioncontroller") || owner.equals("com.google.android.permissioncontroller")) &&
                        node.getText()!=null && "Allow".equalsIgnoreCase(node.getText().toString()) && node.isVisibleToUser() && node.isEnabled() && node.isClickable()){
                        node.performAction(AccessibilityNodeInfo.ACTION_CLICK);
                    }
                }
                Thread.sleep(150);
            }
            require(LocalNetworkAccess.granted(context));
        }
        if(VpnService.prepare(context)==null)return;
        TelecomFixtureProbe.main(instrumentation,()->{context.startActivity(new Intent(context,MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));return null;});
        TelecomFixtureProbe.control(instrumentation,"Allow internet pause",true);
        long deadline=SystemClock.elapsedRealtime()+25000;
        while(SystemClock.elapsedRealtime()<deadline){
            if(VpnService.prepare(context)==null)return;
            AccessibilityNodeInfo root=instrumentation.getUiAutomation().getRootInActiveWindow();
            if(root!=null && !root.findAccessibilityNodeInfosByText("App Control Child").isEmpty()){
                for(String label:new String[]{"Allow","OK"})for(AccessibilityNodeInfo node:root.findAccessibilityNodeInfosByText(label)){
                    String owner=node.getPackageName()==null?"":node.getPackageName().toString();
                    if((owner.equals("com.android.permissioncontroller") || owner.equals("com.google.android.permissioncontroller") || owner.equals("com.android.vpndialogs")) &&
                        node.getText()!=null && label.equalsIgnoreCase(node.getText().toString()) && node.isVisibleToUser() && node.isEnabled() && node.isClickable()){
                        node.performAction(AccessibilityNodeInfo.ACTION_CLICK);
                    }
                }
            }
            Thread.sleep(150);
        }
        throw new IllegalStateException();
    }
    public static Bundle run(Context context,Instrumentation instrumentation,String phase){
        Bundle result=new Bundle();String stage="guard";String vpnAtGuard="not_checked";String managementAtBaseline="not_attempted";String managementWhilePaused="not_attempted";Bundle targetWhilePaused=null;boolean touched=false;
        result.putString("network_pause_ipv6","not-tested");result.putString("network_pause_ipv6_scope","not-tested");
        try{
            stage="guard_fingerprint_and_debug";require(Build.FINGERPRINT.startsWith("google/sdk_gphone64_x86_64/emu64xa:") && BuildConfig.DEBUG);
            stage="guard_unenrolled";
            require(context.getSharedPreferences("enrollment",Context.MODE_PRIVATE).getAll().isEmpty());
            stage="guard_device_owner_and_no_always_on_vpn";
            DevicePolicyManager manager=context.getSystemService(DevicePolicyManager.class);
            ComponentName admin=new ComponentName(context,AppControlDeviceAdminReceiver.class);
            require(manager.isDeviceOwnerApp(context.getPackageName()) && manager.getAlwaysOnVpnPackage(admin)==null);
            stage="guard_existing_vpn_absent";vpnAtGuard=vpnSummary(context);require("cleanup".equals(phase) || "none".equals(vpnAtGuard));
            if("cleanup".equals(phase)){cleanup(context);result.putString("network_pause_probe","cleaned");}
            else{
                stage=baselineFailure(context);require(stage==null);stage="guard_journal_empty";require(context.getSharedPreferences(JOURNAL,Context.MODE_PRIVATE).getAll().isEmpty());
                if("consent".equals(phase)){
                    stage="normal_vpn_consent";consent(context,instrumentation);result.putString("network_pause_probe","ready");
                }else if("run".equals(phase)){
                    require(LocalNetworkAccess.granted(context));
                    stage="fixture_guard";require(VpnService.prepare(context)==null);
                    ApplicationInfo target=context.getPackageManager().getApplicationInfo(TARGET,0);
                    require((target.flags & ApplicationInfo.FLAG_SYSTEM)==0 && (target.flags & ApplicationInfo.FLAG_DEBUGGABLE)!=0);
                    require(context.getPackageManager().checkSignatures(context.getPackageName(),TARGET)==PackageManager.SIGNATURE_MATCH);
                    PolicyEnforcer enforcer=new PolicyEnforcer(context);require(!enforcer.isProtected(TARGET) && enforcer.isProtected(context.getPackageName()));
                    require(!manager.isPackageSuspended(admin,TARGET));
                    JSONArray allowed=new JSONArray();List<InternetPausePlan.App> apps=new ArrayList<>();List<String> allowedNames=new ArrayList<>();
                    for(ApplicationInfo info:context.getPackageManager().getInstalledApplications(0)){
                        boolean system=(info.flags & (ApplicationInfo.FLAG_SYSTEM|ApplicationInfo.FLAG_UPDATED_SYSTEM_APP))!=0;
                        boolean protectedPackage=enforcer.isProtected(info.packageName);
                        apps.add(new InternetPausePlan.App(info.packageName,system,protectedPackage));
                        if(!system && !protectedPackage && !TARGET.equals(info.packageName)){allowed.put(info.packageName);allowedNames.add(info.packageName);}
                    }
                    require(InternetPausePlan.targets(context.getPackageName(),apps,allowedNames).equals(Collections.singleton(TARGET)));
                    stage="baseline_management_packet";managementAtBaseline=managementStatus();require("reachable".equals(managementAtBaseline));stage="baseline_target_fixture";Bundle before=fixture(context);
                    require("reachable".equals(before.getString("network_fixture")) && !before.getBoolean("network_fixture_vpn"));
                    stage="baseline_dns_wire_packets";String baselineDns=DnsWireFixture.status();
                    result.putString("network_pause_dns_baseline_target",before.getString("network_fixture_dns"));
                    result.putString("network_pause_dns_baseline_management",baselineDns);
                    require("reachable".equals(before.getString("network_fixture_dns")) && "reachable".equals(baselineDns));
                    stage="baseline_native_ipv6_packets";String baselineIpv6=NativeIpv6Fixture.status();
                    result.putString("network_pause_ipv6_baseline_target",before.getString("network_fixture_ipv6"));
                    result.putString("network_pause_ipv6_baseline_management",baselineIpv6);
                    boolean ipv6Available="reachable".equals(before.getString("network_fixture_ipv6")) && "reachable".equals(baselineIpv6);
                    if(!ipv6Available){
                        result.putString("network_pause_ipv6", "unproven");
                        result.putString("network_pause_ipv6_scope", "unavailable-baseline");
                    }
                    SharedPreferences stored=cache(context),ledger=context.getSharedPreferences("package-reconciliation",Context.MODE_PRIVATE);
                    SharedPreferences journal=context.getSharedPreferences(JOURNAL,Context.MODE_PRIVATE);
                    require(journal.edit().putBoolean("pending",true).putString("allowed",allowed.toString())
                        .putBoolean("hadAlarmMode",stored.contains("alarmMode")).putString("alarmMode",stored.getString("alarmMode","none"))
                        .putBoolean("hadTracked",ledger.contains("tracked")).commit());
                    stage="pause_apply";touched=true;policy(context,701,true,allowed);PackagePolicyProbe.applied(enforcer.applyCached());
                    stage="pause_network_registration";long networkDeadline=SystemClock.elapsedRealtime()+10000;
                    while("none".equals(vpnSummary(context)) && SystemClock.elapsedRealtime()<networkDeadline)Thread.sleep(100);
                    require(!"none".equals(vpnSummary(context)));
                    stage="paused_management_packet";managementWhilePaused=managementStatus();require("reachable".equals(managementWhilePaused));stage="paused_target_packet";targetWhilePaused=fixture(context);
                    result.putString("network_pause_tcp_paused_target",targetWhilePaused.getString("network_fixture"));
                    require(packetBlocked(targetWhilePaused.getString("network_fixture")) && targetWhilePaused.getBoolean("network_fixture_vpn"));
                    stage="paused_dns_wire_packets";String pausedDns=DnsWireFixture.status();
                    result.putString("network_pause_dns_paused_target",targetWhilePaused.getString("network_fixture_dns"));
                    result.putString("network_pause_dns_paused_management",pausedDns);
                    require(packetBlocked(targetWhilePaused.getString("network_fixture_dns")) && "reachable".equals(pausedDns));
                    if(ipv6Available){
                        stage="paused_native_ipv6_packets";String pausedIpv6=NativeIpv6Fixture.status();
                        result.putString("network_pause_ipv6_paused_target",targetWhilePaused.getString("network_fixture_ipv6"));
                        result.putString("network_pause_ipv6_paused_management",pausedIpv6);
                        require(packetBlocked(targetWhilePaused.getString("network_fixture_ipv6")) && "reachable".equals(pausedIpv6));
                    }
                    require(managementReachable());
                    stage="resume_apply";policy(context,702,false,allowed);PackagePolicyProbe.applied(enforcer.applyCached());
                    stage="resumed_packets";long deadline=SystemClock.elapsedRealtime()+15000;boolean restored=false;
                    while(SystemClock.elapsedRealtime()<deadline){
                        Bundle after=fixture(context);
                        if("reachable".equals(after.getString("network_fixture")) && "reachable".equals(after.getString("network_fixture_dns")) &&
                            !after.getBoolean("network_fixture_vpn") && managementReachable() && "reachable".equals(DnsWireFixture.status()) &&
                            (!ipv6Available || "reachable".equals(after.getString("network_fixture_ipv6")) && "reachable".equals(NativeIpv6Fixture.status()))){restored=true;break;}
                        Thread.sleep(150);
                    }
                    require(restored);result.putString("network_pause_probe","passed");
                    result.putString("network_pause_target","blocked_then_restored");
                    result.putString("network_pause_management","reachable_while_paused");
                    result.putString("network_pause_dns_target","blocked_then_restored");
                    result.putString("network_pause_dns_management","reachable_while_paused");
                    if(ipv6Available){
                        result.putString("network_pause_ipv6","passed");
                        result.putString("network_pause_ipv6_target","blocked_then_restored");
                        result.putString("network_pause_ipv6_management","reachable_while_paused");
                        result.putString("network_pause_ipv6_scope","native-ipv6-fixture-and-management-uid");
                    }
                    result.putString("network_pause_scope","ipv4-tcp-dns-wire-fixture-and-management-uid");
                }else throw new IllegalArgumentException();
            }
        }catch(Exception ignored){result.putString("network_pause_probe","failed");result.putString("network_pause_observation",stage);result.putString("network_pause_tunnel_state",tunnelState());if("guard_existing_vpn_absent".equals(stage))result.putString("network_pause_existing_vpn",vpnAtGuard);if("baseline_management_packet".equals(stage))result.putString("network_pause_management_attempt",managementAtBaseline);if("paused_management_packet".equals(stage))result.putString("network_pause_management_attempt",managementWhilePaused);if("paused_target_packet".equals(stage)&&targetWhilePaused!=null){result.putString("network_pause_target_attempt",targetWhilePaused.getString("network_fixture"));result.putBoolean("network_pause_target_vpn",targetWhilePaused.getBoolean("network_fixture_vpn"));result.putBoolean("network_pause_vpn_available",targetWhilePaused.getBoolean("network_fixture_vpn_available"));}}
        finally{
            if(touched)try{cleanup(context);result.putString("network_pause_cleanup","passed");}
            catch(Exception cleanupFailure){result.putString("network_pause_probe","failed");result.putString("network_pause_cleanup","failed");}
        }
        result.putString("network_pause_version","ipv4-host-gateway-v4");
        return result;
    }
}
