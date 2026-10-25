package org.example.appcontrolfixture;

import android.app.Activity;
import android.os.Bundle;
import android.widget.TextView;

/** Disposable app with no permissions, services or user data. */
public final class FixtureActivity extends Activity {
    @Override public void onCreate(Bundle state) {
        super.onCreate(state);
        TextView text = new TextView(this);
        text.setText("Disposable application for App Control policy tests.");
        setContentView(text);
    }
}
