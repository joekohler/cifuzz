plugins {
    id("java-library")
    id("com.code-intelligence.cifuzz") version "1.5.0"
}

repositories.mavenCentral()

dependencies {
    implementation(project(":app"))
}
