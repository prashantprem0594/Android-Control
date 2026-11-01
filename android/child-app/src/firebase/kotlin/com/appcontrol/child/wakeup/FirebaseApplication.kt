package com.appcontrol.child.wakeup

import android.app.Application
import com.appcontrol.child.BuildConfig
import com.google.firebase.FirebaseApp
import com.google.firebase.FirebaseOptions
import com.google.firebase.messaging.FirebaseMessaging

/** Explicit opt-in build; only public project configuration is compiled. */
class FirebaseApplication : Application() {
    override fun onCreate() {
        super.onCreate()
        val options = FirebaseOptions.Builder()
            .setApplicationId(BuildConfig.FIREBASE_APPLICATION_ID)
            .setApiKey(BuildConfig.FIREBASE_API_KEY)
            .setProjectId(BuildConfig.FIREBASE_PROJECT_ID)
            .setGcmSenderId(BuildConfig.FIREBASE_SENDER_ID).build()
<<<<<<< HEAD
        // FirebaseInitProvider may have already created the default app from
        // the generated resources before Application.onCreate runs.
        if (FirebaseApp.getApps(this).none { it.name == FirebaseApp.DEFAULT_APP_NAME }) {
            FirebaseApp.initializeApp(this, options)
        }
=======
        FirebaseApp.initializeApp(this, options)
>>>>>>> origin/main
        FirebaseMessaging.getInstance().isAutoInitEnabled = true
    }
}
