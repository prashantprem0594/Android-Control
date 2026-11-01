package com.appcontrol.child.remote

import android.accessibilityservice.AccessibilityService
import android.annotation.TargetApi
import android.os.Build
import android.accessibilityservice.GestureDescription
import android.graphics.Bitmap
import android.graphics.Path
import android.hardware.HardwareBuffer
import android.view.Display
import android.view.accessibility.AccessibilityEvent
import java.io.ByteArrayOutputStream
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/** User-enabled Accessibility bridge for the explicitly paired parent account. */
class RemoteAssistanceService : AccessibilityService() {
    override fun onServiceConnected() { Bridge.attach(this) }
    override fun onDestroy() { Bridge.detach(this); super.onDestroy() }
    override fun onAccessibilityEvent(event: AccessibilityEvent?) = Unit
    override fun onInterrupt() = Unit

    object Bridge {
        @Volatile private var service: RemoteAssistanceService? = null
        internal fun attach(value: RemoteAssistanceService) { service = value }
        internal fun detach(value: RemoteAssistanceService) { if (service === value) service = null }

        @TargetApi(30)
        fun captureJpeg(): ByteArray {
            check(Build.VERSION.SDK_INT >= 30) { "Screen snapshots require Android 11 or newer" }
            // A user-started MediaProjection session takes precedence. It provides an explicit,
            // continuously visible sharing boundary and does not depend on accessibility capture.
            RemoteScreenShareService.latestJpeg()?.let { return it }
            val active = checkNotNull(service) { "Remote assistance is not enabled on the child phone" }
            val result = AtomicReference<ByteArray?>()
            val failure = AtomicReference<Throwable?>()
            val complete = CountDownLatch(1)
            active.takeScreenshot(Display.DEFAULT_DISPLAY, active.mainExecutor, object : AccessibilityService.TakeScreenshotCallback {
                override fun onSuccess(screenshot: AccessibilityService.ScreenshotResult) {
                    try {
                    val buffer: HardwareBuffer = screenshot.hardwareBuffer
                    val bitmap = Bitmap.wrapHardwareBuffer(buffer, screenshot.colorSpace)
                    try {
                        checkNotNull(bitmap) { "Could not decode screen capture" }
                        val scale = minOf(1.0, 640.0 / bitmap.width, 640.0 / bitmap.height).toFloat()
                        val reduced = if (scale < 1f) Bitmap.createScaledBitmap(bitmap, (bitmap.width * scale).toInt(), (bitmap.height * scale).toInt(), true) else bitmap
                        try {
                            val output = ByteArrayOutputStream()
                            check(reduced.compress(Bitmap.CompressFormat.JPEG, 60, output)) { "Could not encode screen capture" }
                            result.set(output.toByteArray())
                        } finally { if (reduced !== bitmap) reduced.recycle() }
                    } finally { bitmap?.recycle(); buffer.close() }
                    } catch (error: Throwable) { failure.set(error) } finally { complete.countDown() }
                }
                override fun onFailure(errorCode: Int) {
                    failure.set(IllegalStateException("Screen capture unavailable (Android error $errorCode)"))
                    complete.countDown()
                }
            })
            check(complete.await(10, TimeUnit.SECONDS)) { "Screen capture timed out" }
            failure.get()?.let { throw IllegalStateException("Screen capture failed", it) }
            return checkNotNull(result.get()) { "Screen capture unavailable" }
        }

        fun tap(normalizedX: Int, normalizedY: Int) {
			gesture(normalizedX, normalizedY, normalizedX, normalizedY, 70)
        }

        fun swipe(x: Int, y: Int, endX: Int, endY: Int, durationMillis: Int) = gesture(x, y, endX, endY, durationMillis)
        fun longPress(x: Int, y: Int, durationMillis: Int) = gesture(x, y, x, y, durationMillis)
        fun global(action: String) {
            val active = checkNotNull(service) { "Remote assistance is not enabled on the child phone" }
            val platformAction = when (action) {
                "back" -> AccessibilityService.GLOBAL_ACTION_BACK
                "home" -> AccessibilityService.GLOBAL_ACTION_HOME
                "recents" -> AccessibilityService.GLOBAL_ACTION_RECENTS
                else -> throw IllegalArgumentException("Unsupported remote global action")
            }
            check(active.performGlobalAction(platformAction)) { "System action was rejected" }
        }

        private fun gesture(x: Int, y: Int, endX: Int, endY: Int, durationMillis: Int) {
            require(x in 0..10000 && y in 0..10000 && endX in 0..10000 && endY in 0..10000 && durationMillis in 50..2000)
            val active = checkNotNull(service) { "Remote assistance is not enabled on the child phone" }
            val metrics = active.resources.displayMetrics
            val path = Path().apply {
                moveTo(metrics.widthPixels * x / 10000f, metrics.heightPixels * y / 10000f)
                lineTo(metrics.widthPixels * endX / 10000f, metrics.heightPixels * endY / 10000f)
            }
            val gesture = GestureDescription.Builder().addStroke(GestureDescription.StrokeDescription(path, 0, durationMillis.toLong())).build()
            check(active.dispatchGesture(gesture, null, null)) { "Touch gesture was rejected" }
        }
    }
}
