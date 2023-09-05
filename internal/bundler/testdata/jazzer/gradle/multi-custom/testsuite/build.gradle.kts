plugins {
    id("java-library")
    id("com.code-intelligence.cifuzz") version "1.5.0"
}

sourceSets.getByName("test") {
	java.srcDir("junit-src")
}

repositories.mavenCentral()

dependencies {
    implementation(project(":app"))
}


