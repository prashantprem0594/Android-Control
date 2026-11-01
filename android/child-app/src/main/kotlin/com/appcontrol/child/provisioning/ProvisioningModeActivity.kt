package com.appcontrol.child.provisioning

import android.app.Activity
import android.app.admin.DevicePolicyManager
import android.content.Intent
import android.os.Bundle
<<<<<<< HEAD
import android.os.Build
=======
>>>>>>> origin/main
import android.os.PersistableBundle
import com.appcontrol.child.BuildConfig

/** Invoked by Android's privileged setup wizard, never starts a reset itself. */
class ProvisioningModeActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setResult(RESULT_CANCELED)
        runCatching {
            check(intent.action == DevicePolicyManager.ACTION_GET_PROVISIONING_MODE)
            check(BuildConfig.SERVER_ORIGIN.isNotEmpty())
            val allowed = intent.getIntegerArrayListExtra(DevicePolicyManager.EXTRA_PROVISIONING_ALLOWED_PROVISIONING_MODES)
            check(allowed?.contains(DevicePolicyManager.PROVISIONING_MODE_FULLY_MANAGED_DEVICE) == true)
<<<<<<< HEAD
            val extras = if (Build.VERSION.SDK_INT >= 33) intent.getParcelableExtra(DevicePolicyManager.EXTRA_PROVISIONING_ADMIN_EXTRAS_BUNDLE, PersistableBundle::class.java)
                else intent.getParcelableExtra<PersistableBundle>(DevicePolicyManager.EXTRA_PROVISIONING_ADMIN_EXTRAS_BUNDLE)
=======
            val extras = intent.getParcelableExtra(DevicePolicyManager.EXTRA_PROVISIONING_ADMIN_EXTRAS_BUNDLE, PersistableBundle::class.java)
>>>>>>> origin/main
            val supplied = extras?.getString("appcontrol.serverOrigin")
            check(supplied == null || supplied == BuildConfig.SERVER_ORIGIN)
            setResult(RESULT_OK, Intent().putExtra(DevicePolicyManager.EXTRA_PROVISIONING_MODE, DevicePolicyManager.PROVISIONING_MODE_FULLY_MANAGED_DEVICE))
        }
        finish()
    }
}
