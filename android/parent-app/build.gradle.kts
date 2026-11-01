import org.gradle.api.file.DirectoryProperty
import org.gradle.api.provider.Property
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.OutputDirectory
import org.gradle.api.tasks.TaskAction

plugins {
    id("com.android.application")
}

apply(from = rootProject.file("server-trust.gradle.kts"))
apply(from = rootProject.file("app-version.gradle.kts"))
apply(from = rootProject.file("update-trust.gradle.kts"))

val parentFirebaseEnabled = providers.gradleProperty("appControlParentFirebase").orNull.let {
    require(it == null || it == "true" || it == "false") { "appControlParentFirebase must be true or false" }
    it == "true"
}
val parentFirebaseProperties = mapOf(
    "APPLICATION_ID" to ("appControlParentFirebaseApplicationId" to Regex("1:[0-9]+:android:[a-fA-F0-9]+")),
    "API_KEY" to ("appControlParentFirebaseApiKey" to Regex("[A-Za-z0-9_-]{20,128}")),
    "PROJECT_ID" to ("appControlParentFirebaseProjectId" to Regex("[a-z0-9][a-z0-9-]{4,62}")),
    "SENDER_ID" to ("appControlParentFirebaseSenderId" to Regex("[0-9]{1,32}"))
)
require(parentFirebaseEnabled || parentFirebaseProperties.values.none { providers.gradleProperty(it.first).isPresent }) {
    "Public parent Firebase settings require appControlParentFirebase=true"
}
val parentFirebaseSettings = if (parentFirebaseEnabled) {
    parentFirebaseProperties.mapValues { (_, setting) ->
        val value = providers.gradleProperty(setting.first).orNull
        require(value != null && setting.second.matches(value)) { "Missing or invalid public parent Firebase setting: ${setting.first}" }
        value
    }.also { settings ->
        require(settings.getValue("APPLICATION_ID").split(':')[1] == settings.getValue("SENDER_ID")) {
            "Parent Firebase sender and application IDs differ"
        }
        require(settings.getValue("APPLICATION_ID") != providers.gradleProperty("appControlFirebaseApplicationId").orNull) {
            "Parent and child Firebase application IDs must differ"
        }
    }
} else emptyMap()

android {
    namespace = "com.appcontrol.parent"
    compileSdk = 37
    buildFeatures { buildConfig = true }

    defaultConfig {
        applicationId = "com.appcontrol.parent"
        minSdk = 33
        targetSdk = 37
        versionCode = project.extra["appVersionCode"] as Int
        versionName = project.extra["appVersionName"] as String
        testInstrumentationRunner = "com.appcontrol.transport.ServerTrustInstrumentation"
        buildConfigField("String", "SERVER_ORIGIN", "\"${project.extra["serverTrustOrigin"]}\"")
        buildConfigField("String", "UPDATE_PUBLIC_KEY", "\"${project.extra["updatePublicKey"]}\"")
        buildConfigField("String", "UPDATE_CERTIFICATE_SHA256", "\"${project.extra["updateCertificateSHA256"]}\"")
        buildConfigField("int", "UPDATE_MIN_SEQUENCE", project.extra["updateMinimumSequence"].toString())
        buildConfigField("boolean", "PARENT_PUSH_ENABLED", "false")
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
        if (parentFirebaseEnabled) {
            create("fcmDebug") {
                initWith(getByName("debug"))
                matchingFallbacks += "debug"
                buildConfigField("boolean", "PARENT_PUSH_ENABLED", "true")
                parentFirebaseSettings.forEach { (name, value) -> buildConfigField("String", "PARENT_FIREBASE_$name", "\"$value\"") }
            }
            create("fcmRelease") {
                initWith(getByName("release"))
                matchingFallbacks += "release"
                buildConfigField("boolean", "PARENT_PUSH_ENABLED", "true")
                parentFirebaseSettings.forEach { (name, value) -> buildConfigField("String", "PARENT_FIREBASE_$name", "\"$value\"") }
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    sourceSets.getByName("androidTest").java.srcDir(rootProject.file("shared/src/androidTest/java"))
    sourceSets.getByName("debug").kotlin.directories.add("src/noFirebase/kotlin")
    sourceSets.getByName("release").kotlin.directories.add("src/noFirebase/kotlin")
    if (parentFirebaseEnabled) {
        for (variant in listOf("fcmDebug", "fcmRelease")) {
            sourceSets.getByName(variant).kotlin.directories.add("src/firebase/kotlin")
            sourceSets.getByName(variant).manifest.srcFile("src/firebase/AndroidManifest.xml")
        }
    }
    providers.gradleProperty("appControlArtifactFixtureDir").orNull?.let {
        sourceSets.getByName("androidTest").assets.srcDir(it)
    }
}

if (parentFirebaseEnabled) {
    dependencies {
        add("fcmDebugImplementation", "com.google.firebase:firebase-messaging:26.0.1")
        add("fcmReleaseImplementation", "com.google.firebase:firebase-messaging:26.0.1")
    }
}

abstract class GenerateServerTrustResources : DefaultTask() {
    @get:Input abstract val xmlContent: Property<String>
    @get:Input abstract val developmentCaPem: Property<String>
    @get:OutputDirectory abstract val outputDirectory: DirectoryProperty
    @TaskAction fun generate() {
        val target = outputDirectory.file("xml/server_trust.xml").get().asFile
        target.parentFile.mkdirs()
        target.writeText(xmlContent.get())
        val developmentCa = outputDirectory.file("raw/appcontrol_development_ca.pem").get().asFile
        if (developmentCaPem.get().isNotEmpty()) {
            developmentCa.parentFile.mkdirs()
            developmentCa.writeText(developmentCaPem.get())
        } else if (developmentCa.exists()) {
            check(developmentCa.delete()) { "Cannot remove stale generated development CA resource" }
        }
    }
}
abstract class GenerateFirebaseOptionsResources : DefaultTask() {
    @get:Input abstract val applicationId: Property<String>
    @get:Input abstract val apiKey: Property<String>
    @get:Input abstract val projectId: Property<String>
    @get:Input abstract val senderId: Property<String>
    @get:OutputDirectory abstract val outputDirectory: DirectoryProperty
    @TaskAction fun generate() {
        val target = outputDirectory.file("values/firebase_options.xml").get().asFile
        target.parentFile.mkdirs()
        target.writeText("""<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="google_app_id" translatable="false">${applicationId.get()}</string>
    <string name="google_api_key" translatable="false">${apiKey.get()}</string>
    <string name="project_id" translatable="false">${projectId.get()}</string>
    <string name="gcm_defaultSenderId" translatable="false">${senderId.get()}</string>
</resources>
""")
    }
}
androidComponents.onVariants { variant ->
    variant.sources.java?.addStaticSourceDirectory(rootProject.file("shared/src/main/java").absolutePath)
    val generateTrust = tasks.register<GenerateServerTrustResources>("generate${variant.name.replaceFirstChar { it.uppercaseChar() }}ServerTrust") {
        val development = variant.buildType == "debug" || variant.buildType == "fcmDebug"
        xmlContent.set(project.extra[if (development) "serverTrustDebugXml" else "serverTrustXml"] as String)
        developmentCaPem.set(if (development) project.extra["serverTrustDevelopmentCaPem"] as String else "")
    }
    variant.sources.res?.addGeneratedSourceDirectory(generateTrust, GenerateServerTrustResources::outputDirectory)
    if (variant.name == "fcmDebug" || variant.name == "fcmRelease") {
        val generateFirebaseOptions = tasks.register<GenerateFirebaseOptionsResources>("generate${variant.name.replaceFirstChar { it.uppercaseChar() }}FirebaseOptions") {
            applicationId.set(parentFirebaseSettings.getValue("APPLICATION_ID"))
            apiKey.set(parentFirebaseSettings.getValue("API_KEY"))
            projectId.set(parentFirebaseSettings.getValue("PROJECT_ID"))
            senderId.set(parentFirebaseSettings.getValue("SENDER_ID"))
        }
        variant.sources.res?.addGeneratedSourceDirectory(generateFirebaseOptions, GenerateFirebaseOptionsResources::outputDirectory)
    }
}
