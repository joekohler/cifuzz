package report

import (
	"time"

	"code-intelligence.com/cifuzz/pkg/finding"
)

type Handler interface {
	Handle(report *Report) error
}

type Report struct {
	Status          RunStatus        `json:"status,omitempty"`
	Metric          *FuzzingMetric   `json:"metric,omitempty"`
	Finding         *finding.Finding `json:"finding,omitempty"`
	NumSeeds        uint             `json:"num_seeds,omitempty"`
	SeedCorpus      string           `json:"seed_corpus,omitempty"`
	GeneratedCorpus string           `json:"generated_corpus,omitempty"`
}

func (x *Report) GetFinding() *finding.Finding {
	if x != nil {
		return x.Finding
	}
	return nil
}

type RunStatus string

// These constants must have this exact value (in uppercase) to be able
// to parse JSON-marshalled reports as protobuf reports which use an
// enum for this field.
const (
	RunStatusUnspecified             RunStatus = "UNSPECIFIED"
	RunStatusPending                 RunStatus = "PENDING"
	RunStatusCompiling               RunStatus = "COMPILING"
	RunStatusRunning                 RunStatus = "RUNNING"
	RunStatusStopped                 RunStatus = "STOPPED"
	RunStatusFailed                  RunStatus = "FAILED"
	RunStatusSucceeded               RunStatus = "SUCCEEDED"
	RunStatusUnknown                 RunStatus = "UNKNOWN"
	RunStatusInitializing            RunStatus = "INITIALIZING"
	RunStatusFailedToStart           RunStatus = "FAILED_TO_START"
	RunStatusWaitingForFuzzingAgents RunStatus = "WAITING_FOR_FUZZING_AGENTS"
)

type FuzzingMetric struct {
	Timestamp               time.Time `json:"timestamp,omitempty"`
	ExecutionsPerSecond     int32     `json:"executions_per_second,omitempty"`
	Features                int32     `json:"features,omitempty"`
	CorpusSize              int32     `json:"corpus_size,omitempty"`
	SecondsSinceLastFeature uint64    `json:"seconds_since_last_coverage,omitempty"`
	TotalExecutions         uint64    `json:"total_executions,omitempty"`
	Edges                   int32     `json:"edges,omitempty"`
	SecondsSinceLastEdge    uint64    `json:"seconds_since_last_edge,omitempty"`
}
