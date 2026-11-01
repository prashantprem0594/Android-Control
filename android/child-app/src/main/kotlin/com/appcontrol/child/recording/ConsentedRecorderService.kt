package com.appcontrol.child.recording

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.media.MediaRecorder
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import java.io.File

/**
 * Records only after the person using this phone explicitly opens App Control
 * and approves a short session. Parent commands may operate this service only
 * while that visible session is alive. Audio never leaves the child phone.
 */
class ConsentedRecorderService : Service() {
    private val handler = Handler(Looper.getMainLooper())
    private var recorder: MediaRecorder? = null
    private var recording = false
    private val expiry = Runnable { endSession() }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_APPROVE -> approveSession()
            ACTION_REMOTE_START -> if (sessionActive(this)) startRecording()
            ACTION_REMOTE_STOP -> if (sessionActive(this)) stopRecording()
            ACTION_END -> endSession()
        }
        return START_NOT_STICKY
    }

    private fun approveSession() {
        val until = System.currentTimeMillis() + SESSION_MILLIS
        prefs(this).edit().putLong(SESSION_UNTIL, until).commit()
        showForeground(false)
        handler.removeCallbacks(expiry)
        handler.postDelayed(expiry, SESSION_MILLIS)
    }

    private fun startRecording() {
        if (recording || checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) return
        val destination = File(File(cacheDir, "consented-recordings").also { it.mkdirs() }, "recording-${System.currentTimeMillis()}.m4a")
        val candidate = MediaRecorder().apply {
            setAudioSource(MediaRecorder.AudioSource.MIC)
            setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
            setAudioEncoder(MediaRecorder.AudioEncoder.AAC)
            setOutputFile(destination.absolutePath)
        }
        try {
            candidate.prepare()
            candidate.start()
            recorder = candidate
            recording = true
            showForeground(true)
        } catch (_: Exception) {
            candidate.release()
        }
    }

    private fun stopRecording() {
        val current = recorder ?: return
        recorder = null
        recording = false
        runCatching { current.stop() }
        current.reset()
        current.release()
        if (sessionActive(this)) showForeground(false)
    }

    private fun endSession() {
        handler.removeCallbacks(expiry)
        stopRecording()
        prefs(this).edit().remove(SESSION_UNTIL).commit()
        if (Build.VERSION.SDK_INT >= 24) stopForeground(STOP_FOREGROUND_REMOVE) else @Suppress("DEPRECATION") stopForeground(true)
        stopSelf()
    }

    override fun onDestroy() {
        handler.removeCallbacks(expiry)
        stopRecording()
        super.onDestroy()
    }
    override fun onBind(intent: Intent?): IBinder? = null

    private fun showForeground(isRecording: Boolean) {
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel(CHANNEL, "Approved recorder", NotificationManager.IMPORTANCE_LOW))
        val stop = PendingIntent.getService(this, 0, Intent(this, ConsentedRecorderService::class.java).setAction(ACTION_END),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val notification = Notification.Builder(this, CHANNEL)
            .setSmallIcon(android.R.drawable.ic_btn_speak_now)
            .setContentTitle(if (isRecording) "App Control is recording" else "Recorder control session is active")
            .setContentText(if (isRecording) "Microphone is on. Audio stays on this phone. Tap Stop now to end it." else "A parent may request this app's recorder for a limited time. No recording is active.")
            .setOngoing(true)
            .addAction(Notification.Action.Builder(null, "Stop now", stop).build())
            .build()
        if (Build.VERSION.SDK_INT >= 29) startForeground(NOTIFICATION_ID, notification, android.content.pm.ServiceInfo.FOREGROUND_SERVICE_TYPE_MICROPHONE)
        else @Suppress("DEPRECATION") startForeground(NOTIFICATION_ID, notification)
    }

    companion object {
        private const val CHANNEL = "consented_recorder"
        private const val NOTIFICATION_ID = 7310
        private const val PREFS = "consented-recorder"
        private const val SESSION_UNTIL = "sessionUntil"
        private const val SESSION_MILLIS = 15 * 60 * 1000L
        private const val ACTION_APPROVE = "com.appcontrol.child.recording.APPROVE"
        private const val ACTION_REMOTE_START = "com.appcontrol.child.recording.REMOTE_START"
        private const val ACTION_REMOTE_STOP = "com.appcontrol.child.recording.REMOTE_STOP"
        private const val ACTION_END = "com.appcontrol.child.recording.END"
        private fun prefs(context: Context) = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        fun sessionActive(context: Context): Boolean {
            val until = prefs(context).getLong(SESSION_UNTIL, 0)
            if (until > System.currentTimeMillis()) return true
            if (until != 0L) prefs(context).edit().remove(SESSION_UNTIL).apply()
            return false
        }
        private fun send(context: Context, action: String) = context.startForegroundService(Intent(context, ConsentedRecorderService::class.java).setAction(action))
        fun approve(context: Context) = send(context, ACTION_APPROVE)
        fun requestStart(context: Context) = send(context, ACTION_REMOTE_START)
        fun requestStop(context: Context) = send(context, ACTION_REMOTE_STOP)
        fun end(context: Context) = context.startService(Intent(context, ConsentedRecorderService::class.java).setAction(ACTION_END))
    }
}
