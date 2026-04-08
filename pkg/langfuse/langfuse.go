package langfuse

import (
	"context"
	"time"

	"github.com/bytedance/sonic"
	lf "github.com/cloudwego/eino-ext/callbacks/langfuse"
	"github.com/go-resty/resty/v2"
	"github.com/gookit/slog"
)

type Langfuse struct {
	pk   string
	sk   string
	host string
}

type AsyncTaskTraceNamer func(taskType string) string

func New(pk string, sk string, host string) *Langfuse {
	return &Langfuse{
		pk:   pk,
		sk:   sk,
		host: host,
	}
}

func NewAsyncTaskContextBuilder(traceNamer AsyncTaskTraceNamer) func(taskType, userID, sessionID string) context.Context {
	return func(taskType, userID, sessionID string) context.Context {
		name := ""
		if traceNamer != nil {
			name = traceNamer(taskType)
		}

		opts := []lf.TraceOption{
			lf.WithUserID(userID),
			lf.WithSessionID(sessionID),
		}
		if name != "" {
			opts = append(opts, lf.WithName(name))
		}

		return lf.SetTrace(context.Background(), opts...)
	}
}

func (l *Langfuse) GetPrompt(promptName string) string {
	resp, err := resty.New().R().
		SetBasicAuth(l.pk, l.sk).
		Get(l.host + "/api/public/v2/prompts/" + promptName)

	if err != nil {
		slog.Errorf("get prompt fail,err:%s", err)
		return ""
	}

	res := &GetPromptResponse{}
	err = sonic.Unmarshal(resp.Body(), res)
	if err != nil {
		slog.Errorf("unmarshal prompt fail,err:%s", err)
		return ""
	}

	return res.Prompt
}

type GetPromptResponse struct {
	Id        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	ProjectId string    `json:"projectId"`
	CreatedBy string    `json:"createdBy"`
	Prompt    string    `json:"prompt"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	Type      string    `json:"type"`
	IsActive  any       `json:"isActive"`
	Config    struct {
	} `json:"config"`
	Tags            []any    `json:"tags"`
	Labels          []string `json:"labels"`
	CommitMessage   any      `json:"commitMessage"`
	ResolutionGraph any      `json:"resolutionGraph"`
}
