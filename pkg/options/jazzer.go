package options

const (
	JazzerMainClass string = "com.code_intelligence.jazzer.Jazzer"

	JazzerTargetClass  string = "--target_class"
	JazzerTargetMethod string = "--target_method"
	JazzerAutoFuzz     string = "--autofuzz"

	JazzerTargetClassManifest string = "Jazzer-Fuzz-Target-Class"
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
