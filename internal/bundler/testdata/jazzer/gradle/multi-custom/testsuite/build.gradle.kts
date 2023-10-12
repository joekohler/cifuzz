plugins {
    id("java-library")
    id("com.code-intelligence.cifuzz") version "1.6.0"
}

sourceSets.getByName("test") {
	java.srcDir("junit-src")
}

repositories.mavenCentral()

dependencies {
    implementation(project(":app"))
}


