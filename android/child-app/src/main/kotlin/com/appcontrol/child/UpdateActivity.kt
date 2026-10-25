package com.appcontrol.child

class UpdateActivity : com.appcontrol.transport.ReleaseActivity() {
    override fun updatePublicKey() = BuildConfig.UPDATE_PUBLIC_KEY
    override fun updateOrigin() = BuildConfig.SERVER_ORIGIN
    override fun updateCertificate() = BuildConfig.UPDATE_CERTIFICATE_SHA256
    override fun updateMinimum() = BuildConfig.UPDATE_MIN_SEQUENCE.toLong()
}
