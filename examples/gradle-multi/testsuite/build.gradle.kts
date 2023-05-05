plugins {
    id("java-library")
    id("com.code-intelligence.cifuzz") version "1.1.1"
}

repositories.mavenCentral()

dependencies {
    implementation(project(":app"))
}
