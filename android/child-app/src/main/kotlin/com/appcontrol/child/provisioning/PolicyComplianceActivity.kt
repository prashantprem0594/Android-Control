package com.appcontrol.child.provisioning

import android.app.Activity
import android.app.admin.DevicePolicyManager
import android.os.Bundle
<<<<<<< HEAD
import android.os.Build
=======
>>>>>>> origin/main
import android.os.PersistableBundle
import com.appcontrol.child.BuildConfig

/** Confirms actual ownership only. Enrollment and user permissions stay explicit. */
class PolicyComplianceActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setResult(RESULT_CANCELED)
        runCatching {
            check(intent.action == DevicePolicyManager.ACTION_ADMIN_POLICY_COMPLIANCE)
            check(BuildConfig.SERVER_ORIGIN.isNotEmpty())
            check(getSystemService(DevicePolicyManager::class.java).isDeviceOwnerApp(packageName))
<<<<<<< HEAD
            val extras = if (Build.VERSION.SDK_INT >= 33) intent.getParcelableExtra(DevicePolicyManager.EXTRA_PROVISIONING_ADMIN_EXTRAS_BUNDLE, PersistableBundle::class.java)
                else intent.getParcelableExtra<PersistableBundle>(DevicePolicyManager.EXTRA_PROVISIONING_ADMIN_EXTRAS_BUNDLE)
=======
            val extras = intent.getParcelableExtra(DevicePolicyManager.EXTRA_PROVISIONING_ADMIN_EXTRAS_BUNDLE, PersistableBundle::class.java)
>>>>>>> origin/main
            val supplied = extras?.getString("appcontrol.serverOrigin")
            check(supplied == null || supplied == BuildConfig.SERVER_ORIGIN)
            check(getSharedPreferences("provisioning-state", MODE_PRIVATE).edit()
                .putBoolean("deviceOwnerConfirmed", true).putString("server", BuildConfig.SERVER_ORIGIN).commit())
            setResult(RESULT_OK)
        }
        finish()
    }
}
