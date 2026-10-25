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

val firebaseEnabled = providers.gradleProperty("appControlFirebase").orNull.let {
    require(it == null || it == "true" || it == "false") { "appControlFirebase must be true or false" }
    it == "true"
}
val firebaseSettings = if (firebaseEnabled) {
    mapOf(
        "APPLICATION_ID" to ("appControlFirebaseApplicationId" to Regex("1:[0-9]+:android:[a-fA-F0-9]+")),
        "API_KEY" to ("appControlFirebaseApiKey" to Regex("[A-Za-z0-9_-]{20,128}")),
        "PROJECT_ID" to ("appControlFirebaseProjectId" to Regex("[a-z0-9][a-z0-9-]{4,62}")),
        "SENDER_ID" to ("appControlFirebaseSenderId" to Regex("[0-9]{1,32}"))
    ).mapValues { (_, setting) ->
        val value = providers.gradleProperty(setting.first).orNull
        require(value != null && setting.second.matches(value)) { "Missing or invalid public Firebase setting: ${setting.first}" }
        value
    }.also { require(it.getValue("APPLICATION_ID").split(':')[1] == it.getValue("SENDER_ID")) { "Firebase sender and application IDs differ" } }
} else emptyMap()
val remoteCallsEnabled = providers.gradleProperty("appControlRemoteCallsEnabled").orNull.let {
    require(it == null || it == "true" || it == "false") { "appControlRemoteCallsEnabled must be true or false" }
    it == "true"
}

android {
    namespace = "com.appcontrol.child"
    compileSdk = 37
    buildFeatures { buildConfig = true }

    defaultConfig {
        applicationId = "com.appcontrol.child"
        minSdk = 33
        targetSdk = 37
        versionCode = project.extra["appVersionCode"] as Int
        versionName = project.extra["appVersionName"] as String
        testInstrumentationRunner = "com.appcontrol.transport.ServerTrustInstrumentation"
        buildConfigField("String", "SERVER_ORIGIN", "\"${project.extra["serverTrustOrigin"]}\"")
        buildConfigField("String", "UPDATE_PUBLIC_KEY", "\"${project.extra["updatePublicKey"]}\"")
        buildConfigField("String", "UPDATE_CERTIFICATE_SHA256", "\"${project.extra["updateCertificateSHA256"]}\"")
        buildConfigField("int", "UPDATE_MIN_SEQUENCE", project.extra["updateMinimumSequence"].toString())
        buildConfigField("boolean", "REMOTE_CALLS_ENABLED", remoteCallsEnabled.toString())
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
        if (firebaseEnabled) {
            create("fcmDebug") {
                initWith(getByName("debug"))
                matchingFallbacks += "debug"
                firebaseSettings.forEach { (name, value) -> buildConfigField("String", "FIREBASE_$name", "\"$value\"") }
            }
            create("fcmRelease") {
                initWith(getByName("release"))
                matchingFallbacks += "release"
                firebaseSettings.forEach { (name, value) -> buildConfigField("String", "FIREBASE_$name", "\"$value\"") }
            }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    sourceSets.getByName("androidTest").java.srcDir(rootProject.file("shared/src/androidTest/java"))
    sourceSets.getByName("androidTest").java.srcDir("src/test/java")
    sourceSets.getByName("debug").manifest.srcFile("src/debug/AndroidManifest.xml")
    sourceSets.getByName("debug").kotlin.directories.add("src/noFirebase/kotlin")
    sourceSets.getByName("release").kotlin.directories.add("src/noFirebase/kotlin")
    if (firebaseEnabled) {
        for (variant in listOf("fcmDebug", "fcmRelease")) {
            sourceSets.getByName(variant).kotlin.directories.add("src/firebase/kotlin")
            sourceSets.getByName(variant).manifest.srcFile("src/firebase/AndroidManifest.xml")
        }
    }
}

if (firebaseEnabled) {
    dependencies {
        add("fcmDebugImplementation", "com.google.firebase:firebase-messaging:26.0.1")
        add("fcmReleaseImplementation", "com.google.firebase:firebase-messaging:26.0.1")
    }
}

abstract class GenerateServerTrustResources : DefaultTask() {
    @get:Input abstract val xmlContent: Property<String>
    @get:OutputDirectory abstract val outputDirectory: DirectoryProperty
    @TaskAction fun generate() {
        val target = outputDirectory.file("xml/server_trust.xml").get().asFile
        target.parentFile.mkdirs()
        target.writeText(xmlContent.get())
    }
}
androidComponents.onVariants { variant ->
    variant.sources.java?.addStaticSourceDirectory(rootProject.file("shared/src/main/java").absolutePath)
    val generateTrust = tasks.register<GenerateServerTrustResources>("generate${variant.name.replaceFirstChar { it.uppercaseChar() }}ServerTrust") {
        xmlContent.set(project.extra["serverTrustXml"] as String)
    }
    variant.sources.res?.addGeneratedSourceDirectory(generateTrust, GenerateServerTrustResources::outputDirectory)
}
