plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "io.github.felipaocosta.leftoff"
    compileSdk = 36

    defaultConfig {
        applicationId = "io.github.felipaocosta.leftoff"
        minSdk = 24
        targetSdk = 36
        versionCode = 1
        versionName = (project.findProperty("appVersion") as String?) ?: "dev"
    }

    // The release key never lives in the repository: android/build.sh reads
    // it from LEFTOFF_* variables (a signing.env kept outside the tree).
    // Without them, release builds are signed with the debug key (local
    // development only; such an APK must not be distributed).
    val releaseKeystore = System.getenv("LEFTOFF_KEYSTORE")
    signingConfigs {
        if (releaseKeystore != null) {
            create("release") {
                storeFile = file(releaseKeystore)
                storePassword = System.getenv("LEFTOFF_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("LEFTOFF_KEY_ALIAS")
                keyPassword = System.getenv("LEFTOFF_KEY_PASSWORD")
                storeType = "pkcs12"
            }
        }
    }
    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.findByName("release") ?: signingConfigs.getByName("debug")
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    // The Go core (mobile/), built with gomobile bind by android/build.sh.
    implementation(files("libs/leftoff.aar"))
    implementation("androidx.activity:activity-ktx:1.10.1")
    implementation("androidx.core:core-ktx:1.16.0")
    implementation("androidx.core:core-splashscreen:1.0.1")
    implementation("androidx.swiperefreshlayout:swiperefreshlayout:1.1.0")
    implementation("androidx.work:work-runtime-ktx:2.10.1")
}
