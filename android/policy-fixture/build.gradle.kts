plugins { id("com.android.application") }
android {
    namespace = "org.example.appcontrolfixture"
    compileSdk = 37
    defaultConfig {
        applicationId = "org.example.appcontrolfixture"
        minSdk = 33
        targetSdk = 37
        versionCode = 1
        versionName = "1.0-fixture"
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}
