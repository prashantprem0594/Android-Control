package org.example.appcontrolnetworkfixture;

/** Fixed transport categories only; exception text and peer details never leave the fixture. */
final class PacketFailure {
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
