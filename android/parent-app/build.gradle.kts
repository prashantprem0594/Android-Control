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
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    sourceSets.getByName("androidTest").java.srcDir(rootProject.file("shared/src/androidTest/java"))
    providers.gradleProperty("appControlArtifactFixtureDir").orNull?.let {
        sourceSets.getByName("androidTest").assets.srcDir(it)
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
