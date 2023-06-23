package options

const (
	JazzerMainClass string = "com.code_intelligence.jazzer.Jazzer"

	JazzerTargetClass  string = "--target_class"
	JazzerTargetMethod string = "--target_method"
	JazzerAutoFuzz     string = "--autofuzz"

	// we keep that for compatibility reasons,
	// can be removed when we are sure that there
	// are no more jazzer versions < 0.19.0 around
	JazzerTargetClassManifestLegacy string = "Jazzer-Fuzz-Target-Class"
	JazzerTargetClassManifest       string = "Jazzer-Target-Class"
	JazzerTargetMethodManifest      string = "Jazzer-Target-Method"
)

func JazzerTargetClassFlag(value string) string {
	return JazzerTargetClass + "=" + value
}

func JazzerTargetMethodFlag(value string) string {
	return JazzerTargetMethod + "=" + value
}

func JazzerAutoFuzzFlag(value string) string {
	return JazzerAutoFuzz + "=" + value
}
