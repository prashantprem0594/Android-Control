// Shared public version metadata. Defaults preserve the initial development build.
val suppliedCode = providers.gradleProperty("appControlVersionCode").orNull
val suppliedName = providers.gradleProperty("appControlVersionName").orNull
require((suppliedCode == null) == (suppliedName == null)) { "Supply version code and name together" }
val code = suppliedCode?.toIntOrNull() ?: if (suppliedCode == null) 1 else error("Invalid app version code")
require(code in 1..2100000000 && (suppliedCode == null || code.toString() == suppliedCode)) { "Invalid app version code" }
val name = suppliedName ?: "0.1.0"
require(name.matches(Regex("[A-Za-z0-9][A-Za-z0-9._+\\-]{0,63}"))) { "Invalid app version name" }
extra["appVersionCode"] = code
extra["appVersionName"] = name
