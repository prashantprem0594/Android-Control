package com.appcontrol.parent.testing

import android.content.Context
import android.util.AtomicFile
import org.json.JSONObject
import java.io.File
import java.io.RandomAccessFile

/** Debug-only cross-process fixture state. Baseline preferences are never diagnostic output. */
object ParentQueueDeathState {
    const val VERSION = "parent-queue-death-v1"
    private val lock = Any()
    private fun file(context: Context, baseline: Boolean) = File(context.filesDir,
        if (baseline) "parent-queue-death-baseline.json" else "parent-queue-death-state.json")
    private fun <T> locked(context: Context, action: () -> T): T = synchronized(lock) {
        RandomAccessFile(File(context.filesDir, "parent-queue-death.lock"), "rw").channel.use { channel ->
            channel.lock().use { action() }
        }
    }
    private fun readFile(context: Context, baseline: Boolean): JSONObject {
        val bytes = AtomicFile(file(context, baseline)).readFully()
        check(bytes.size in 2..131_072)
        return JSONObject(bytes.toString(Charsets.UTF_8)).also { check(it.getString("version") == VERSION) }
    }
    private fun writeFile(context: Context, value: JSONObject, baseline: Boolean) {
        val bytes = value.toString().toByteArray(Charsets.UTF_8)
        check(bytes.size <= 131_072)
        val atomic = AtomicFile(file(context, baseline))
        val output = atomic.startWrite()
        try { output.write(bytes); atomic.finishWrite(output) }
        catch (failure: Exception) { atomic.failWrite(output); throw failure }
    }
    fun exists(context: Context, baseline: Boolean = false): Boolean = locked(context) {
        file(context, baseline).exists() || File(file(context, baseline).path + ".bak").exists()
    }
    fun create(context: Context, value: JSONObject, baseline: Boolean = false) = locked(context) {
        check(!file(context, baseline).exists())
        writeFile(context, value, baseline)
    }
    fun read(context: Context, baseline: Boolean = false): JSONObject = locked(context) { readFile(context, baseline) }
    fun update(context: Context, change: (JSONObject) -> Unit): JSONObject = locked(context) {
        readFile(context, false).also { change(it); writeFile(context, it, false) }
    }
    fun remove(context: Context) = locked(context) {
        AtomicFile(file(context, false)).delete()
        AtomicFile(file(context, true)).delete()
        check(!file(context, false).exists() && !file(context, true).exists())
    }
}
