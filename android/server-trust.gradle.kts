import java.net.URI
import java.io.ByteArrayInputStream
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.nio.charset.StandardCharsets
import java.util.Base64
import org.gradle.api.DefaultTask
import org.gradle.api.provider.Property
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.TaskAction

abstract class ValidateReleaseTrust : DefaultTask() {
    @get:Input abstract val hasConfiguration: Property<Boolean>
    @get:Input abstract val hasDevelopmentCa: Property<Boolean>
    @TaskAction fun validate() {
        check(!hasDevelopmentCa.get()) { "Development CA trust is debug-only and must never be packaged in a release" }
        check(hasConfiguration.get()) { "Release builds require server origin and distinct current/backup SHA-256 pins" }
    }
}

// Shared by both apps. Values are public deployment metadata, never keys.
val trustOrigin = providers.gradleProperty("appControlServerOrigin").orNull?.trim().orEmpty()
val trustCurrent = providers.gradleProperty("appControlCurrentPin").orNull?.trim().orEmpty()
val trustBackup = providers.gradleProperty("appControlBackupPin").orNull?.trim().orEmpty()
val configured = listOf(trustOrigin, trustCurrent, trustBackup).any { it.isNotEmpty() }
val developmentCaPath = providers.gradleProperty("appControlDevelopmentCaFile").orNull?.trim().orEmpty()
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

// The temporary laptop root is a public debug-only trust anchor, scoped to the
// configured host. It never bypasses pinning or changes hostname verification.
val developmentCaPem = if (developmentCaPath.isNotEmpty()) {
    require(configured) { "Development CA trust requires configured HTTPS origin and both pins" }
    val pemBytes = runCatching {
        val source = file(developmentCaPath)
        require(source.isFile && source.length() in 1..65536) { "Invalid development CA certificate file" }
        providers.fileContents(layout.projectDirectory.file(source.absolutePath)).asBytes.get()
    }.getOrElse { error("Cannot read a bounded public development CA certificate") }
    require(pemBytes.size in 1..65536) { "Invalid development CA certificate size" }
    val pem = pemBytes.toString(StandardCharsets.US_ASCII)
    val pemMatch = Regex("\\s*-----BEGIN CERTIFICATE-----\\r?\\n([A-Za-z0-9+/=\\r\\n]+)-----END CERTIFICATE-----\\s*").matchEntire(pem)
    require(pemMatch != null) {
        "Development CA must contain exactly one public PEM certificate and no private key or extra blocks"
    }
    val body = pemMatch.groupValues[1].replace("\r", "").replace("\n", "")
    val certificateDer = runCatching { Base64.getDecoder().decode(body) }
        .getOrElse { error("Invalid public development CA certificate encoding") }
    require(Base64.getEncoder().encodeToString(certificateDer) == body) { "Development CA certificate must use canonical Base64" }
    val certificate = runCatching {
        (CertificateFactory.getInstance("X.509").generateCertificate(ByteArrayInputStream(certificateDer)) as X509Certificate).also {
            require(it.encoded.contentEquals(certificateDer)) { "Development CA contains extra certificate data" }
            it.checkValidity()
            require(it.basicConstraints >= 0) { "Development certificate is not a CA" }
            val usage = it.keyUsage
            require(usage == null || usage.size > 5 && usage[5]) { "Development CA cannot sign certificates" }
        }
    }.getOrElse { error("Development trust requires a valid public X.509 CA certificate") }
    "-----BEGIN CERTIFICATE-----\n" +
        Base64.getMimeEncoder(64, byteArrayOf(10)).encodeToString(certificate.encoded) +
        "\n-----END CERTIFICATE-----\n"
} else ""
require(developmentCaPem.isEmpty() || gradle.startParameter.taskNames.none { it.contains("release", ignoreCase = true) }) {
    "Development CA trust is debug-only and must never be supplied to release tasks"
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
extra["serverTrustDevelopmentCaPem"] = developmentCaPem
extra["serverTrustDebugXml"] = if (developmentCaPem.isEmpty()) extra["serverTrustXml"] as String else {
    val developmentDomain = domain.replace("        <pin-set>", """        <trust-anchors>
            <certificates src="system" overridePins="false" />
            <certificates src="@raw/appcontrol_development_ca" overridePins="false" />
        </trust-anchors>
        <pin-set>""")
    """<?xml version="1.0" encoding="utf-8"?>
<network-security-config>
    <base-config cleartextTrafficPermitted="false">
        <trust-anchors><certificates src="system" /></trust-anchors>
    </base-config>$developmentDomain
</network-security-config>
"""
}
val validateReleaseServerTrust = tasks.register<ValidateReleaseTrust>("validateReleaseServerTrust") {
    hasConfiguration.set(configured)
    hasDevelopmentCa.set(developmentCaPem.isNotEmpty())
}
extra["serverTrustOrigin"] = canonicalOrigin
tasks.configureEach {
    if (name.contains("Release") && name != "validateReleaseServerTrust") dependsOn(validateReleaseServerTrust)
}
