package cron_test

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	cronPkg "github.com/CoolBanHub/aggo/cron"
	agmsg "github.com/CoolBanHub/aggo/internal/agentic"
	cronTool "github.com/CoolBanHub/aggo/tools/cron"
	"github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// scriptedAgentModel 第一轮返回工具调用，之后返回纯文本结束对话。
type scriptedAgentModel struct {
	mu       sync.Mutex
	toolCall *schema.AgenticMessage
	inputs   []*schema.AgenticMessage
}

func (m *scriptedAgentModel) Generate(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.AgenticMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs = input
	tc := m.toolCall
	if tc != nil {
		m.toolCall = nil
		return tc, nil
	}
	return agmsg.AssistantMessage("好的"), nil
}

func (m *scriptedAgentModel) Stream(ctx context.Context, input []*schema.AgenticMessage, opts ...einomodel.Option) (*schema.StreamReader[*schema.AgenticMessage], error) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	r, w := schema.Pipe[*schema.AgenticMessage](1)
	go func() {
		defer w.Close()
		w.Send(msg, nil)
	}()
	return r, nil
}

func toolCallMessage(args map[string]any) *schema.AgenticMessage {
	raw, _ := json.Marshal(args)
	msg := agmsg.AssistantMessage("")
	msg.ContentBlocks = []*schema.ContentBlock{
		schema.NewContentBlock(&schema.FunctionToolCall{
			CallID:    "call-1",
			Name:      "cron",
			Arguments: string(raw),
		}),
	}
	return msg
}

func newTestService(t *testing.T) *cronPkg.CronService {
	t.Helper()
	store, err := cronPkg.NewFileStoreWithError(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	return cronPkg.NewCronService(store, nil)
}

func runAgent(t *testing.T, service *cronPkg.CronService, args map[string]any, session map[string]any) (*scriptedAgentModel, error) {
	t.Helper()
	cm := &scriptedAgentModel{toolCall: toolCallMessage(args)}
	agent, err := adk.NewTypedChatModelAgent[*schema.AgenticMessage](context.Background(),
		&adk.TypedChatModelAgentConfig[*schema.AgenticMessage]{
			Name:        "cron-test",
			Description: "test",
			Instruction: "test",
			Model:       cm,
			ToolsConfig: adk.ToolsConfig{
				ToolsNodeConfig: compose.ToolsNodeConfig{Tools: cronTool.GetTools(service)},
			},
		})
	if err != nil {
		t.Fatal(err)
	}
	runner := adk.NewTypedRunner[*schema.AgenticMessage](adk.TypedRunnerConfig[*schema.AgenticMessage]{Agent: agent})

	var opts []adk.AgentRunOption
	if session != nil {
		opts = append(opts, adk.WithSessionValues(session))
	}
	var runErr error
	iter := runner.Run(context.Background(), []*schema.AgenticMessage{schema.UserAgenticMessage("执行工具")}, opts...)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil && event.Err != io.EOF {
			runErr = event.Err
		}
	}
	return cm, runErr
}

// 会话缺少身份时，list 操作默认拒绝，不会返回全量任务。
func TestToolDeniesWhenIdentityMissing(t *testing.T) {
	service := newTestService(t)

	// 预置两个用户的任务
	for _, user := range []string{"userA", "userB"} {
		every := int64(3600)
		if _, err := service.AddJob("job", cronPkg.CronSchedule{Kind: "every", EveryMS: &every}, "msg", user); err != nil {
			t.Fatal(err)
		}
	}

	cm, runErr := runAgent(t, service, map[string]any{"action": "list"}, nil)

	// 工具返回的错误会中止本轮运行，其中应包含拒绝原因
	if runErr == nil || !strings.Contains(runErr.Error(), "无法从会话中解析用户身份") {
		t.Fatalf("expected identity-missing denial, got runErr=%v inputs=%s", runErr, allText(cm.inputs))
	}
}

// 用户 A 通过工具只能看到自己的任务，不能删除/禁用用户 B 的任务。
func TestToolEnforcesOwnershipAcrossUsers(t *testing.T) {
	service := newTestService(t)
	every := int64(3600)
	jobA, err := service.AddJob("a-job", cronPkg.CronSchedule{Kind: "every", EveryMS: &every}, "msg-a", "userA")
	if err != nil {
		t.Fatal(err)
	}
	jobB, err := service.AddJob("b-job", cronPkg.CronSchedule{Kind: "every", EveryMS: &every}, "msg-b", "userB")
	if err != nil {
		t.Fatal(err)
	}

	sessionA := map[string]any{"userID": "userA", "sessionID": "sess-a"}
	sessionB := map[string]any{"userID": "userB", "sessionID": "sess-b"}

	// A 列表只含自己的任务（工具调用参数不含任务 ID，可整体断言无泄漏）
	cm, runErr := runAgent(t, service, map[string]any{"action": "list"}, sessionA)
	if runErr != nil {
		t.Fatalf("userA list should succeed: %v", runErr)
	}
	text := allText(cm.inputs)
	if !strings.Contains(text, jobA.ID) {
		t.Fatalf("userA list result missing own job: %s", text)
	}
	if strings.Contains(text, jobB.ID) || strings.Contains(text, "msg-b") {
		t.Fatalf("userA list result leaked userB job: %s", text)
	}

	// A 尝试删除 B 的任务 → 拒绝，任务仍在
	_, runErr = runAgent(t, service, map[string]any{"action": "remove", "job_id": jobB.ID}, sessionA)
	if runErr == nil || !strings.Contains(runErr.Error(), "not found") {
		t.Fatalf("userA removing userB job should be denied, got: %v", runErr)
	}
	if !serviceHasJob(t, service, jobB.ID) {
		t.Fatal("userB job must survive denied remove")
	}

	// A 尝试禁用 B 的任务 → 拒绝，任务仍启用
	_, runErr = runAgent(t, service, map[string]any{"action": "disable", "job_id": jobB.ID}, sessionA)
	if runErr == nil || !strings.Contains(runErr.Error(), "not found") {
		t.Fatalf("userA disabling userB job should be denied, got: %v", runErr)
	}
	if !jobEnabled(t, service, jobB.ID) {
		t.Fatal("userB job must remain enabled")
	}

	// B 删除自己的任务 → 成功
	cm, runErr = runAgent(t, service, map[string]any{"action": "remove", "job_id": jobB.ID}, sessionB)
	if runErr != nil {
		t.Fatalf("userB removing own job should succeed: %v", runErr)
	}
	if serviceHasJob(t, service, jobB.ID) {
		t.Fatal("userB job should be removed")
	}
}

// 同一群聊（conversationID/sessionID 相同）中不同 actor 的任务互不可见。
func TestToolSeparatesActorsInSameConversation(t *testing.T) {
	service := newTestService(t)
	every := int64(3600)
	if _, err := service.AddJob("zhang", cronPkg.CronSchedule{Kind: "every", EveryMS: &every}, "msg", "zhang"); err != nil {
		t.Fatal(err)
	}
	liJob, err := service.AddJob("li", cronPkg.CronSchedule{Kind: "every", EveryMS: &every}, "msg", "li")
	if err != nil {
		t.Fatal(err)
	}

	cm, runErr := runAgent(t, service, map[string]any{"action": "list"}, map[string]any{
		"actorID":        "zhang",
		"conversationID": "chat-1",
		"sessionID":      "chat-1",
	})
	if runErr != nil {
		t.Fatalf("zhang list should succeed: %v", runErr)
	}
	if text := allText(cm.inputs); strings.Contains(text, liJob.ID) {
		t.Fatalf("actor zhang must not see actor li's job in the same chat: %s", text)
	}
}

func allText(messages []*schema.AgenticMessage) string {
	var b strings.Builder
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		b.WriteString(agmsg.Text(msg))
		b.WriteString("\n")
	}
	return b.String()
}

func serviceHasJob(t *testing.T, service *cronPkg.CronService, jobID string) bool {
	t.Helper()
	for _, user := range []string{"userA", "userB", "zhang", "li"} {
		jobs, err := service.ListJobsByUser(user, true)
		if err != nil {
			continue
		}
		for _, job := range jobs {
			if job.ID == jobID {
				return true
			}
		}
	}
	return false
}

func jobEnabled(t *testing.T, service *cronPkg.CronService, jobID string) bool {
	t.Helper()
	for _, user := range []string{"userA", "userB", "zhang", "li"} {
		jobs, err := service.ListJobsByUser(user, true)
		if err != nil {
			continue
		}
		for _, job := range jobs {
			if job.ID == jobID {
				return job.Enabled
			}
		}
	}
	t.Fatalf("job %s not found", jobID)
	return false
}
