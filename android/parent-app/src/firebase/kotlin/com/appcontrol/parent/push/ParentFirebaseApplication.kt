package com.appcontrol.parent.push

import android.app.Application
import com.appcontrol.parent.BuildConfig
import com.google.firebase.FirebaseApp
import com.google.firebase.FirebaseOptions
import com.google.firebase.messaging.FirebaseMessaging

/** Explicit Firebase variant; only the separate parent's public configuration is compiled. */
class ParentFirebaseApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        val options = FirebaseOptions.Builder()
            .setApplicationId(BuildConfig.PARENT_FIREBASE_APPLICATION_ID)
            .setApiKey(BuildConfig.PARENT_FIREBASE_API_KEY)
            .setProjectId(BuildConfig.PARENT_FIREBASE_PROJECT_ID)
            .setGcmSenderId(BuildConfig.PARENT_FIREBASE_SENDER_ID)
            .build()
        // FirebaseInitProvider may have already created the default app from
        // the generated resources before Application.onCreate runs.
        if (FirebaseApp.getApps(this).none { it.name == FirebaseApp.DEFAULT_APP_NAME }) {
            FirebaseApp.initializeApp(this, options)
        }
        // Only the eligible authenticated worker explicitly requests a token.
        FirebaseMessaging.getInstance().isAutoInitEnabled = false
    }
}
