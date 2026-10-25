package org.example.appcontroltelecomfixture;

import android.net.Uri;
import android.telecom.Connection;
import android.telecom.ConnectionRequest;
import android.telecom.ConnectionService;
import android.telecom.DisconnectCause;
import android.telecom.PhoneAccountHandle;
import android.telecom.TelecomManager;

/** In-process synthetic SIP state only; no media implementation or network. */
public final class FixtureConnectionService extends ConnectionService {
    static volatile Connection current;
    static volatile String observation = "idle";
    static volatile boolean expected;
    static void finish(int cause) {
        Connection connection = current; current = null;
        if (connection != null) { connection.setDisconnected(new DisconnectCause(cause)); connection.destroy(); }
        observation = "disconnected";
    }
    @Override public Connection onCreateIncomingConnection(PhoneAccountHandle account, ConnectionRequest request) {
        if (!expected || current != null) return Connection.createFailedConnection(new DisconnectCause(DisconnectCause.ERROR));
        expected = false;
        Connection connection = new Connection() {
            @Override public void onAnswer() { setActive(); observation = "active"; }
            @Override public void onAnswer(int videoState) { onAnswer(); }
            @Override public void onReject() { finish(DisconnectCause.REJECTED); }
            @Override public void onDisconnect() { finish(DisconnectCause.LOCAL); }
            @Override public void onHold() { setOnHold(); observation = "holding"; }
            @Override public void onUnhold() { setActive(); observation = "active"; }
        };
        connection.setConnectionCapabilities(Connection.CAPABILITY_HOLD | Connection.CAPABILITY_SUPPORT_HOLD | Connection.CAPABILITY_MUTE);
        connection.setAudioModeIsVoip(true);
        connection.setAddress(Uri.fromParts("sip", "fixture.invalid", null), TelecomManager.PRESENTATION_RESTRICTED);
        connection.setCallerDisplayName("Synthetic fixture", TelecomManager.PRESENTATION_ALLOWED);
        current = connection; observation = "ringing"; connection.setRinging(); return connection;
    }
    @Override public void onCreateIncomingConnectionFailed(PhoneAccountHandle account, ConnectionRequest request) { expected = false; observation = "failed"; }
    @Override public Connection onCreateOutgoingConnection(PhoneAccountHandle account, ConnectionRequest request) {
        return Connection.createFailedConnection(new DisconnectCause(DisconnectCause.ERROR));
    }
}
