plugins {
    id("java-library")
    id("com.code-intelligence.cifuzz") version "1.4.0"
}

repositories.mavenCentral()

dependencies {
    implementation(project(":app"))
}
