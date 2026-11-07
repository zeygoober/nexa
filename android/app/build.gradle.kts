import java.util.Properties
import javax.inject.Inject
import org.gradle.process.ExecOperations

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.plugin.compose")
    id("org.jetbrains.kotlin.plugin.serialization")
}

/**
 * Release signing is configured locally in android/signing. The keystore and its passwords
 * are never committed; see signing/README.md. A debug build never needs them.
 */
val keystoreProps = Properties().apply {
    val f = rootProject.file("signing/release.properties")
    if (f.exists()) f.inputStream().use { load(it) }
}

/**
 * Strings, sounds, the emoji sprites and What's New are the web client's own files, copied
 * into the app's assets at build time, so text translated once is translated
 * everywhere and a sound changed once changes everywhere.
 */
abstract class CopyWebAssets : DefaultTask() {
    @get:InputDirectory abstract val locales: DirectoryProperty
    @get:InputDirectory abstract val sounds: DirectoryProperty
    @get:InputDirectory abstract val sprites: DirectoryProperty
    @get:InputDirectory abstract val seasonal: DirectoryProperty
    @get:InputFile abstract val whatsNew: RegularFileProperty
    @get:InputDirectory abstract val legalDocs: DirectoryProperty
    @get:InputFile abstract val legalVersion: RegularFileProperty
    @get:OutputDirectory abstract val outputDir: DirectoryProperty

    @TaskAction
    fun copy() {
        val out = outputDir.get().asFile
        out.deleteRecursively()
        fun copyDir(from: File, to: String, keep: (File) -> Boolean) {
            val into = out.resolve(to)
            into.mkdirs()
            from.listFiles()?.filter { it.isFile && keep(it) }?.forEach { it.copyTo(into.resolve(it.name), overwrite = true) }
        }
        copyDir(locales.get().asFile, "locales") { it.name.endsWith(".json") }
        copyDir(sounds.get().asFile, "sounds") { it.name.endsWith(".m4a") }
        copyDir(sprites.get().asFile, "twemoji") { it.name.endsWith(".webp") || it.name.endsWith(".json") }
        // the pictures seasonal themes sit on the message box (ui/theme/look/Seasonal.kt)
        copyDir(seasonal.get().asFile, "seasonal") { it.name.endsWith(".svg") }
        // What's New is the web's own list, read by the app (ui/WhatsNew.kt), so a release is written once
        out.resolve("whatsnew").mkdirs()
        whatsNew.get().asFile.copyTo(out.resolve("whatsnew/whatsNew.ts"), overwrite = true)
        // the Terms and Privacy Policy, and which version of them people agree to (lib/legal.ts)
        copyDir(legalDocs.get().asFile, "legal") { it.name.endsWith(".txt") }
        legalVersion.get().asFile.copyTo(out.resolve("legal/legal.ts"), overwrite = true)
    }
}

val copyWebAssets = tasks.register<CopyWebAssets>("copyWebAssets") {
    locales.set(rootProject.file("../web/src/i18n/locales"))
    sounds.set(rootProject.file("../web/public/sounds"))
    sprites.set(rootProject.file("../web/src/assets/twemoji-sprites"))
    seasonal.set(rootProject.file("../web/src/assets/seasonal"))
    whatsNew.set(rootProject.file("../web/src/lib/whatsNew.ts"))
    legalDocs.set(rootProject.file("../docs/legal"))
    legalVersion.set(rootProject.file("../web/src/lib/legal.ts"))
}

/**
 * The notes CRDT (android/ydoc): yrs, the Rust port of the Yjs the web keeps
 * notes in, built with cargo-ndk for each processor the app ships for, so a
 * phone and a browser editing one note exchange the very same updates.
 */
abstract class BuildYdoc : DefaultTask() {
    @get:InputDirectory abstract val sources: DirectoryProperty
    @get:InputFile abstract val manifest: RegularFileProperty
    @get:InputFile abstract val lockfile: RegularFileProperty
    @get:Input abstract val ndkDir: Property<String>
    @get:OutputDirectory abstract val outputDir: DirectoryProperty
    @get:Inject abstract val exec: ExecOperations

    @TaskAction
    fun build() {
        val home = System.getProperty("user.home")
        val cargoBin = File(home, ".cargo/bin")
        val cargo = File(cargoBin, "cargo").takeIf { it.exists() }?.path ?: "cargo"
        val out = outputDir.get().asFile
        out.deleteRecursively()
        exec.exec {
            workingDir = manifest.get().asFile.parentFile
            environment("ANDROID_NDK_HOME", ndkDir.get())
            environment("PATH", cargoBin.path + File.pathSeparator + (System.getenv("PATH") ?: ""))
            commandLine(cargo, "ndk", "-t", "arm64-v8a", "-t", "armeabi-v7a", "-t", "x86_64", "-P", "26", "-o", out.path, "build", "--release")
        }
    }
}

val buildYdoc = tasks.register<BuildYdoc>("buildYdoc") {
    sources.set(rootProject.file("ydoc/src"))
    manifest.set(rootProject.file("ydoc/Cargo.toml"))
    lockfile.set(rootProject.file("ydoc/Cargo.lock"))
    ndkDir.set(androidComponents.sdkComponents.ndkDirectory.map { it.asFile.path })
}

androidComponents {
    onVariants { variant ->
        variant.sources.assets?.addGeneratedSourceDirectory(copyWebAssets, CopyWebAssets::outputDir)
        variant.sources.jniLibs?.addGeneratedSourceDirectory(buildYdoc, BuildYdoc::outputDir)
    }
}

android {
    namespace = "fish.nexa"
    compileSdk = 37
    compileSdkMinor = 2
    ndkVersion = "29.0.14206865"

    defaultConfig {
        applicationId = "fish.nexa"
        minSdk = 26
        targetSdk = 36
        // tools/android_release.py numbers each release after the one it published last
        // (-PnexaVersionCode, -PnexaVersionName); these are what a build without them carries
        versionCode = (findProperty("nexaVersionCode") as String?)?.trim()?.toIntOrNull() ?: 3
        versionName = (findProperty("nexaVersionName") as String?)?.trim()?.takeIf { it.isNotEmpty() } ?: "1.1.0"

        // -PserverUrl=http://127.0.0.1:18555 points a build at a local chatd (through adb reverse)
        val server = (findProperty("serverUrl") as String?)?.trim().takeUnless { it.isNullOrEmpty() } ?: "https://nexa.pm"
        buildConfigField("String", "DEFAULT_SERVER", "\"$server\"")
        // -PallSeasons=true: every seasonal theme in season every day, for trying them out (never for a release)
        buildConfigField("boolean", "ALL_SEASONS", (findProperty("allSeasons") as String?)?.trim().equals("true").toString())

        ndk {
            abiFilters += listOf("arm64-v8a", "armeabi-v7a", "x86_64")
        }
        externalNativeBuild {
            cmake {
                cppFlags += listOf("-std=c++17", "-fno-rtti", "-fno-exceptions", "-O2", "-fvisibility=hidden")
                cFlags += listOf("-O2", "-fvisibility=hidden")
            }
        }
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    externalNativeBuild {
        cmake {
            path = file("src/main/cpp/CMakeLists.txt")
            version = "3.31.6"
        }
    }

    signingConfigs {
        if (keystoreProps.containsKey("storeFile")) {
            create("release") {
                storeFile = rootProject.file(keystoreProps.getProperty("storeFile"))
                storePassword = keystoreProps.getProperty("storePassword")
                keyAlias = keystoreProps.getProperty("keyAlias")
                keyPassword = keystoreProps.getProperty("keyPassword")
            }
        }
    }

    // One build per processor keeps each download to about a third; the
    // universal one is what the website hands a phone it cannot place.
    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "armeabi-v7a", "x86_64")
            isUniversalApk = true
        }
    }

    buildTypes {
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            // A release signed with anything but the release key could never
            // update an installed copy, so it fails rather than falls back.
            signingConfig = signingConfigs.findByName("release")
                ?: if (gradle.startParameter.taskNames.any { it.contains("Release", ignoreCase = true) }) {
                    error("a release build needs android/signing/release.properties and the keystore it names")
                } else {
                    signingConfigs.getByName("debug")
                }
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    packaging {
        resources {
            excludes += setOf("META-INF/{AL2.0,LGPL2.1}", "META-INF/DEPENDENCIES", "META-INF/versions/9/OSGI-INF/MANIFEST.MF", "META-INF/*.kotlin_module")
        }
        jniLibs {
            // compressed in the APK: a smaller download, unpacked once at install
            useLegacyPackaging = true
        }
    }

    testOptions {
        unitTests.isReturnDefaultValues = true
    }

    lint {
        abortOnError = false
        checkReleaseBuilds = false
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
        freeCompilerArgs.addAll(
            "-opt-in=androidx.compose.foundation.ExperimentalFoundationApi",
            "-opt-in=androidx.compose.ui.ExperimentalComposeUiApi",
            "-opt-in=androidx.compose.animation.ExperimentalAnimationApi",
            "-opt-in=kotlinx.coroutines.ExperimentalCoroutinesApi",
            "-opt-in=kotlinx.serialization.ExperimentalSerializationApi",
            "-Xcontext-parameters",
        )
    }
}

dependencies {
    val composeBom = platform("androidx.compose:compose-bom:2026.09.00")
    implementation(composeBom)

    implementation("androidx.core:core-ktx:1.19.0")
    implementation("androidx.core:core-splashscreen:1.0.1")
    implementation("androidx.activity:activity-compose:1.13.0")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.11.0")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.11.0")
    implementation("androidx.lifecycle:lifecycle-process:2.11.0")
    implementation("androidx.lifecycle:lifecycle-service:2.11.0")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-graphics")
    implementation("androidx.compose.ui:ui-text-google-fonts")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.animation:animation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.ui:ui-tooling-preview")
    debugImplementation("androidx.compose.ui:ui-tooling")

    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.11.0")
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.11.0")
    implementation("org.jetbrains.kotlinx:kotlinx-collections-immutable:0.5.2")
    implementation("com.squareup.okhttp3:okhttp:5.5.0")
    // QR codes of safety numbers, and reading the ones a login page shows
    implementation("com.google.zxing:core:3.5.3")
    implementation("androidx.camera:camera-camera2:1.4.2")
    implementation("androidx.camera:camera-lifecycle:1.4.2")
    implementation("androidx.camera:camera-view:1.4.2")
    // passkeys, through the system's own credential manager (Android 14 and newer, no Play services)
    implementation("androidx.credentials:credentials:1.5.0")

    implementation("io.coil-kt.coil3:coil-compose:3.6.3")
    implementation("io.coil-kt.coil3:coil-gif:3.6.3")
    implementation("io.coil-kt.coil3:coil-svg:3.6.3")
    implementation("io.coil-kt.coil3:coil-network-okhttp:3.6.3")
    implementation("io.coil-kt.coil3:coil-video:3.6.3")

    implementation("androidx.media3:media3-exoplayer:1.11.1")
    implementation("androidx.media3:media3-datasource-okhttp:1.11.1")
    // the video surface as a composable (PlayerSurface); the controls are drawn in Nexa's own look
    implementation("androidx.media3:media3-ui-compose:1.11.1")
    // Lottie stickers (.tgs is gzipped Lottie JSON)
    implementation("com.airbnb.android:lottie-compose:6.7.1")

    implementation("org.bouncycastle:bcprov-jdk18on:1.86")
    implementation("com.lambdapioneer.argon2kt:argon2kt:1.6.0")

    implementation("io.github.webrtc-sdk:android:150.7871.01")

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.11.0")
    testImplementation("org.json:json:20250517")
    // the ICU the web's own Node was built with, standing in for the phone's android.icu in JVM tests
    testImplementation("com.ibm.icu:icu4j:78.3")
}
