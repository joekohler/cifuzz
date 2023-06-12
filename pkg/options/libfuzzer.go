package options

const (
	LibFuzzerMaxTotalTime   string = "-max_total_time"
	LibFuzzerDictionary     string = "-dict"
	LibFuzzerArtifactPrefix string = "-artifact_prefix"
)

func LibFuzzerMaxTotalTimeFlag(value string) string {
	return LibFuzzerMaxTotalTime + "=" + value
}

func LibFuzzerDictionaryFlag(value string) string {
	return LibFuzzerDictionary + "=" + value
}

func LibFuzzerArtifactPrefixFlag(value string) string {
	return LibFuzzerArtifactPrefix + "=" + value
}
