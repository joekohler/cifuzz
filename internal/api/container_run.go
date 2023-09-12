package api

type ContainerRun struct {
	Image     string     `json:"image"`
	FuzzTests []FuzzTest `json:"fuzz_tests,omitempty"`

	// ProjectNid is the new project ID format used in responses from CI Sense.
	// Future responses from the /v3 API will use these nano IDs and are usually
	// prefixed with "prj", "job", "org", etc.
	ProjectNid string `json:"project_nid,omitempty"`

	// ProjectExternalID is the conventional project id used in the URL of CI Sense
	ProjectExternalID string `json:"project_external_id,omitempty"`
}

type ContainerRunResponse struct {
	Run   *Run   `json:"run,omitempty"`
	Links []Link `json:"links,omitempty"`
}

type FuzzTest struct {
	Name string `json:"name"`
	Jobs []Job  `json:"jobs,omitempty"`
}

type Link struct {
	Href   string `json:"href,omitempty"`
	Rel    string `json:"rel,omitempty"`
	Method string `json:"method,omitempty"`
}

type Run struct {
	Nid       string     `json:"nid,omitempty"`
	Image     string     `json:"image,omitempty"`
	FuzzTests []FuzzTest `json:"fuzz_tests,omitempty"`
}
type Job struct {
	Nid    string `json:"nid,omitempty"`
	Type   string `json:"type,omitempty"`
	Status string `json:"status,omitempty"`
	Config string `json:"config,omitempty"`
}
