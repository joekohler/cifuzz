plugins {
    id("java-library")
    id("com.code-intelligence.cifuzz") version "1.6.0"
}

repositories.mavenCentral()

tasks.test {
    useJUnitPlatform()
}

dependencies {
    implementation(project(":app"))
}

sourceSets.getByName("test") {
    java.srcDir("fuzzTests")
}
