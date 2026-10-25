import java.net.URI
import java.util.Base64
import org.gradle.api.DefaultTask
import org.gradle.api.provider.Property
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.TaskAction

abstract class ValidateReleaseTrust : DefaultTask() {
    @get:Input abstract val hasConfiguration: Property<Boolean>
    @TaskAction fun validate() {
        check(hasConfiguration.get()) { "Release builds require server origin and distinct current/backup SHA-256 pins" }
    }
}

// Shared by both apps. Values are public deployment metadata, never keys.
val trustOrigin = providers.gradleProperty("appControlServerOrigin").orNull?.trim().orEmpty()
val trustCurrent = providers.gradleProperty("appControlCurrentPin").orNull?.trim().orEmpty()
val trustBackup = providers.gradleProperty("appControlBackupPin").orNull?.trim().orEmpty()
val configured = listOf(trustOrigin, trustCurrent, trustBackup).any { it.isNotEmpty() }
var canonicalOrigin = ""
var trustHost = ""
if (configured) {
    require(listOf(trustOrigin, trustCurrent, trustBackup).all { it.isNotEmpty() }) {
        "Server trust requires origin, current pin and backup pin together"
    }
    val origin = runCatching { URI(trustOrigin) }.getOrElse { error("Invalid server trust origin") }
    val host = origin.host?.lowercase().orEmpty()
    require(origin.scheme == "https" && origin.rawUserInfo == null && origin.rawQuery == null &&
        origin.rawFragment == null && (origin.rawPath.isNullOrEmpty() || origin.rawPath == "/") &&
        (origin.port == -1 || origin.port in 1..65535) && host.length in 1..253 &&
        host.split('.').all { it.length in 1..63 && it.matches(Regex("[a-z0-9](?:[a-z0-9-]*[a-z0-9])?")) } &&
        host.any { it in 'a'..'z' }) { "Server trust requires an HTTPS DNS origin without credentials, path or query" }
    for (pin in listOf(trustCurrent, trustBackup)) {
        val bytes = runCatching { Base64.getDecoder().decode(pin) }.getOrElse { error("Pins must be canonical SHA-256 Base64") }
        require(bytes.size == 32 && Base64.getEncoder().encodeToString(bytes) == pin) {
            "Pins must be canonical SHA-256 Base64"
        }
    }
    require(trustCurrent != trustBackup) { "Current and backup pins must identify distinct keys" }
    trustHost = host
    canonicalOrigin = URI("https", null, host, if (origin.port == 443) -1 else origin.port, null, null, null).toASCIIString()
}

val domain = if (configured) """
    <domain-config cleartextTrafficPermitted="false">
        <domain includeSubdomains="false">$trustHost</domain>
        <pin-set>
            <pin digest="SHA-256">$trustCurrent</pin>
            <pin digest="SHA-256">$trustBackup</pin>
        </pin-set>
    </domain-config>""" else ""
extra["serverTrustXml"] = """<?xml version="1.0" encoding="utf-8"?>
<network-security-config>
    <base-config cleartextTrafficPermitted="false">
        <trust-anchors><certificates src="system" /></trust-anchors>
    </base-config>$domain
</network-security-config>
"""
val validateReleaseServerTrust = tasks.register<ValidateReleaseTrust>("validateReleaseServerTrust") {
    hasConfiguration.set(configured)
}
extra["serverTrustOrigin"] = canonicalOrigin
tasks.configureEach {
    if (name.contains("Release") && name != "validateReleaseServerTrust") dependsOn(validateReleaseServerTrust)
}
