import java.security.KeyFactory
import java.security.spec.X509EncodedKeySpec
import java.util.Base64

// Public update authority is compiled independently of downloaded metadata.
val updateKey = providers.gradleProperty("appControlUpdatePublicKey").orNull.orEmpty()
val updateCertificate = providers.gradleProperty("appControlUpdateCertificateSHA256").orNull.orEmpty()
val minimumText = providers.gradleProperty("appControlUpdateMinimumSequence").orNull ?: "0"
val updateMinimum = minimumText.toLongOrNull() ?: error("Invalid update minimum sequence")
require(minimumText.matches(Regex("0|[1-9][0-9]{0,9}")) && updateMinimum in 0..2100000000L) { "Invalid update minimum sequence" }
if (updateKey.isNotEmpty() || updateCertificate.isNotEmpty()) {
    require(updateKey.isNotEmpty() && updateCertificate.matches(Regex("[a-f0-9]{64}"))) { "Update trust requires public key and lowercase APK certificate digest together" }
    require((extra["serverTrustOrigin"] as String).isNotEmpty()) { "Update trust requires configured HTTPS server trust" }
    val bytes = runCatching { Base64.getDecoder().decode(updateKey) }.getOrElse { error("Invalid update public key") }
    require(bytes.size == 44 && Base64.getEncoder().encodeToString(bytes) == updateKey) { "Update key must be canonical Ed25519 DER Base64" }
    val parsed = runCatching { KeyFactory.getInstance("Ed25519").generatePublic(X509EncodedKeySpec(bytes)) }.getOrElse { error("Invalid update public key") }
    require(parsed.encoded.contentEquals(bytes)) { "Noncanonical update public key" }
} else require(updateMinimum == 0L) { "Update sequence requires configured update trust" }
extra["updatePublicKey"] = updateKey
extra["updateCertificateSHA256"] = updateCertificate
extra["updateMinimumSequence"] = updateMinimum.toInt()
