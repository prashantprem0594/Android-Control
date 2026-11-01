package com.appcontrol.child.remote

import android.app.Activity
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.ColorSpace
import android.hardware.display.DisplayManager
import android.hardware.display.VirtualDisplay
import android.media.ImageReader
import android.media.projection.MediaProjection
import android.media.projection.MediaProjectionManager
import android.os.Build
import android.os.Handler
import android.os.HandlerThread
import android.os.IBinder
import java.io.ByteArrayOutputStream
import java.util.concurrent.atomic.AtomicReference

/**
 * A user-started, visible Android screen-share session.  Frames stay in memory until the
 * existing authenticated command channel asks for a snapshot; this service never starts from
 * a parent command and Android's system prompt is required for every session.
 */
class RemoteScreenShareService : Service() {
    private var projection: MediaProjection? = null
    private var display: VirtualDisplay? = null
    private var reader: ImageReader? = null
    private var thread: HandlerThread? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopSelf()
            return START_NOT_STICKY
        }
        val resultData = intent?.getParcelableExtra<Intent>(EXTRA_RESULT_DATA)
        val resultCode = intent?.getIntExtra(EXTRA_RESULT_CODE, Activity.RESULT_CANCELED) ?: Activity.RESULT_CANCELED
        if (resultCode != Activity.RESULT_OK || resultData == null) {
            stopSelf()
            return START_NOT_STICKY
        }
        startForeground(NOTIFICATION_ID, notification())
        stopCapture()
        val manager = getSystemService(MediaProjectionManager::class.java)
        projection = checkNotNull(manager.getMediaProjection(resultCode, resultData)) { "Android did not grant screen sharing" }.also { capture ->
            capture.registerCallback(object : MediaProjection.Callback() {
                override fun onStop() { stopSelf() }
            }, Handler(mainLooper))
        }
        val width = resources.displayMetrics.widthPixels.coerceAtLeast(1)
        val height = resources.displayMetrics.heightPixels.coerceAtLeast(1)
        thread = HandlerThread("remote-screen-share").also { it.start() }
        reader = ImageReader.newInstance(width, height, android.graphics.PixelFormat.RGBA_8888, 2).also { imageReader ->
            imageReader.setOnImageAvailableListener({ source ->
                source.acquireLatestImage()?.use { image ->
                    val buffer = image.hardwareBuffer ?: return@use
                    val bitmap = Bitmap.wrapHardwareBuffer(buffer, ColorSpace.get(ColorSpace.Named.SRGB)) ?: return@use
                    try { latestFrame.set(encode(bitmap)) } finally { bitmap.recycle() }
                }
            }, Handler(thread!!.looper))
        }
        display = projection!!.createVirtualDisplay(
            "App Control live screen share", width, height, resources.displayMetrics.densityDpi,
            DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR, reader!!.surface, null, Handler(thread!!.looper)
        )
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        stopCapture()
        latestFrame.set(null)
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun stopCapture() {
        display?.release(); display = null
        reader?.close(); reader = null
        projection?.stop(); projection = null
        thread?.quitSafely(); thread = null
    }

    private fun notification(): Notification {
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel(CHANNEL_ID, "Screen sharing", NotificationManager.IMPORTANCE_LOW))
        val stop = PendingIntent.getService(this, 0, Intent(this, RemoteScreenShareService::class.java).setAction(ACTION_STOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        return Notification.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.presence_video_online)
            .setContentTitle("Screen sharing is active")
            .setContentText("Your paired parent can view the visible screen. Tap Stop to end sharing.")
            .setOngoing(true)
            .addAction(Notification.Action.Builder(null, "Stop", stop).build())
            .build()
    }

    private fun encode(bitmap: Bitmap): ByteArray {
        val scale = minOf(1.0, 960.0 / bitmap.width, 960.0 / bitmap.height).toFloat()
        val outputBitmap = if (scale < 1f) Bitmap.createScaledBitmap(bitmap, (bitmap.width * scale).toInt(), (bitmap.height * scale).toInt(), true) else bitmap
        return try {
            ByteArrayOutputStream().use { output ->
                check(outputBitmap.compress(Bitmap.CompressFormat.JPEG, 70, output)) { "Could not encode screen share frame" }
                output.toByteArray()
            }
        } finally { if (outputBitmap !== bitmap) outputBitmap.recycle() }
    }

    companion object {
        private const val CHANNEL_ID = "remote_screen_share"
        private const val NOTIFICATION_ID = 7301
        private const val ACTION_STOP = "com.appcontrol.child.remote.STOP_SCREEN_SHARE"
        private const val EXTRA_RESULT_CODE = "resultCode"
        private const val EXTRA_RESULT_DATA = "resultData"
        private val latestFrame = AtomicReference<ByteArray?>()

        fun latestJpeg(): ByteArray? = latestFrame.get()

        fun start(context: Context, resultCode: Int, data: Intent) {
            val launch = Intent(context, RemoteScreenShareService::class.java)
                .putExtra(EXTRA_RESULT_CODE, resultCode)
                .putExtra(EXTRA_RESULT_DATA, data)
            context.startForegroundService(launch)
        }

        fun stop(context: Context) { context.stopService(Intent(context, RemoteScreenShareService::class.java)) }
    }
}
