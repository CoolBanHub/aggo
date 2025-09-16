package langfuse

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/gookit/slog"
	"gopkg.in/resty.v1"
)

type Langfuse struct {
	pk   string
	sk   string
	host string
}

func New(pk string, sk string, host string) *Langfuse {
	return &Langfuse{
		pk:   pk,
		sk:   sk,
		host: host,
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
