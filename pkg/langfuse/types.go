package langfuse

import "time"

const (
	eventTypeTraceCreate      = "trace-create"
	eventTypeGenerationCreate = "generation-create"
	eventTypeGenerationUpdate = "generation-update"
	eventTypeSpanCreate       = "span-create"
	eventTypeSpanUpdate       = "span-update"

	defaultEnvironment = "default"
)

type ingestionRequest struct {
	Batch    []ingestionEvent `json:"batch"`
	Metadata any              `json:"metadata,omitempty"`
}

type ingestionEvent struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Body      any    `json:"body"`
	Metadata  any    `json:"metadata,omitempty"`
}

type ingestionResponse struct {
	Successes []ingestionResult `json:"successes"`
	Errors    []ingestionResult `json:"errors"`
}

type ingestionResult struct {
	ID      string `json:"id"`
	Status  int    `json:"status"`
	Message string `json:"message,omitempty"`
	Error   any    `json:"error,omitempty"`
}

type traceBody struct {
	ID          string   `json:"id,omitempty"`
	Timestamp   string   `json:"timestamp,omitempty"`
	Name        string   `json:"name,omitempty"`
	UserID      string   `json:"userId,omitempty"`
	Input       any      `json:"input,omitempty"`
	Output      any      `json:"output,omitempty"`
	SessionID   string   `json:"sessionId,omitempty"`
	Release     string   `json:"release,omitempty"`
	Version     string   `json:"version,omitempty"`
	Metadata    any      `json:"metadata,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Environment string   `json:"environment,omitempty"`
	Public      *bool    `json:"public,omitempty"`
}

type observationBody struct {
	ID                  string             `json:"id,omitempty"`
	TraceID             string             `json:"traceId,omitempty"`
	Name                string             `json:"name,omitempty"`
	StartTime           string             `json:"startTime,omitempty"`
	EndTime             string             `json:"endTime,omitempty"`
	CompletionStartTime string             `json:"completionStartTime,omitempty"`
	Model               string             `json:"model,omitempty"`
	ModelParameters     map[string]any     `json:"modelParameters,omitempty"`
	Usage               *usageBody         `json:"usage,omitempty"`
	UsageDetails        map[string]int     `json:"usageDetails,omitempty"`
	CostDetails         map[string]float64 `json:"costDetails,omitempty"`
	PromptName          string             `json:"promptName,omitempty"`
	PromptVersion       *int               `json:"promptVersion,omitempty"`
	Metadata            any                `json:"metadata,omitempty"`
	Input               any                `json:"input,omitempty"`
	Output              any                `json:"output,omitempty"`
	Level               string             `json:"level,omitempty"`
	StatusMessage       string             `json:"statusMessage,omitempty"`
	ParentObservationID string             `json:"parentObservationId,omitempty"`
	Version             string             `json:"version,omitempty"`
	Environment         string             `json:"environment,omitempty"`
}

type usageBody struct {
	Input  int    `json:"input,omitempty"`
	Output int    `json:"output,omitempty"`
	Total  int    `json:"total,omitempty"`
	Unit   string `json:"unit,omitempty"`
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
