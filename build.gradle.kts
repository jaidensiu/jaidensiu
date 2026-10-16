plugins {
    kotlin("jvm") version "2.4.21"
    application
    id("org.graalvm.buildtools.native") version "1.1.14"
}

repositories {
    mavenCentral()
}

dependencies {
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:1.11.0")
}

kotlin {
    jvmToolchain(25)
}

application {
    mainClass = "profile.MainKt"
}

graalvmNative {
    binaries.named("main") {
        imageName = "profile-card"
        buildArgs.addAll("--no-fallback", "--enable-https")
    }
}
