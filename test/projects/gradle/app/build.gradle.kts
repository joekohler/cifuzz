plugins {
    id("java-library")
    id("com.code-intelligence.cifuzz") version "1.3.0"
}

repositories.mavenCentral()

tasks.test {
    useJUnitPlatform()
}
dependencies {
    testImplementation("org.junit.jupiter:junit-jupiter:5.9.2")
}
