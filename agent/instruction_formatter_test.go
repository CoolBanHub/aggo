package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// mockSkillTool mocks the skill tool for testing.
type mockSkillTool struct {
	name    string
	desc    string
	runFunc func(ctx context.Context, args string) (string, error)
}

func (m *mockSkillTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: m.name,
		Desc: m.desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"skill": {Type: schema.String, Desc: "skill name", Required: true},
		}),
	}, nil
}

func (m *mockSkillTool) InvokableRun(ctx context.Context, args string, _ ...tool.Option) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, args)
	}
	return "", nil
}

func TestFormatInstruction_NoFrameworkSections(t *testing.T) {
	input := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁"
	got := formatInstruction(input)
	if got != input {
		t.Errorf("expected unchanged, got:\n%s", got)
	}
}

func TestFormatInstruction_WithTransfer(t *testing.T) {
	input := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁\n\nAvailable other agents: \n- Agent name: cron\n  Agent description: 定时任务助手\n\nDecision rule:\n- ANSWER\n- CALL function"
	got := formatInstruction(input)

	wantBase := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁"
	if !contains(got, wantBase) {
		t.Errorf("base instruction should be preserved, got:\n%s", got)
	}
	if !contains(got, "<available_agents>") {
		t.Error("expected <available_agents> tag")
	}
	if !contains(got, "</available_agents>") {
		t.Error("expected </available_agents> closing tag")
	}
	if !contains(got, "Agent name: cron") {
		t.Error("expected transfer content inside tag")
	}
}

func TestFormatInstruction_WithTransferAndSkill(t *testing.T) {
	input := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁\n\nAvailable other agents: \n- Agent name: cron\n  Agent description: 定时任务助手\n\nDecision rule:\n- ANSWER\n\n# Skills System\n\n**How to Use Skills**\n\nSome instructions here."
	got := formatInstruction(input)

	if !contains(got, "<available_agents>") {
		t.Error("expected <available_agents> tag")
	}
	if !contains(got, "</available_agents>") {
		t.Error("expected </available_agents> closing tag")
	}
	if !contains(got, "<skills_system>") {
		t.Error("expected <skills_system> tag")
	}
	if !contains(got, "</skills_system>") {
		t.Error("expected </skills_system> closing tag")
	}
	if !contains(got, "How to Use Skills") {
		t.Error("expected skill content inside tag")
	}
}

func TestFormatInstruction_Chinese(t *testing.T) {
	input := "你是一个智能助手。\n\n可用的其他 agent：\n- Agent 名字: cron\n  Agent 描述: 定时任务\n\n决策规则：\n- ANSWER\n\n# Skill 系统\n\n使用说明"
	got := formatInstruction(input)

	if !contains(got, "<available_agents>") {
		t.Error("expected <available_agents> tag for Chinese marker")
	}
	if !contains(got, "<skills_system>") {
		t.Error("expected <skills_system> tag for Chinese marker")
	}
}

func TestFormatInstruction_OnlySkill(t *testing.T) {
	input := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁\n\n# Skills System\n\nSome skill content"
	got := formatInstruction(input)

	if contains(got, "<available_agents>") {
		t.Error("should not have available_agents tag when no sub-agents")
	}
	if !contains(got, "<skills_system>") {
		t.Error("expected <skills_system> tag")
	}
}

// --- Runtime skill detection tests ---

func TestHasAvailableSkills_NoTools(t *testing.T) {
	got := hasAvailableSkills(context.Background(), nil)
	if got {
		t.Error("expected false when no tools")
	}
}

func TestHasAvailableSkills_EmptySkills(t *testing.T) {
	tools := []tool.BaseTool{
		&mockSkillTool{
			name: "skill",
			desc: `<skills_instructions>...</skills_instructions><available_skills></available_skills>`,
		},
	}
	got := hasAvailableSkills(context.Background(), tools)
	if got {
		t.Error("expected false when no skills in tool description")
	}
}

func TestHasAvailableSkills_WithSkills(t *testing.T) {
	tools := []tool.BaseTool{
		&mockSkillTool{
			name: "skill",
			desc: `<skills_instructions>...</skills_instructions>
<available_skills>
<skill>
<name>
web-research
</name>
<description>
Research the web
</description>
</skill>
</available_skills>`,
		},
	}
	got := hasAvailableSkills(context.Background(), tools)
	if !got {
		t.Error("expected true when skills exist in tool description")
	}
}

func TestRemoveSkillSection(t *testing.T) {
	input := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁\n\n# Skills System\n\n**How to Use Skills**\n\nSome instructions here."
	got := removeSkillSection(input)

	want := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁"
	if got != want {
		t.Errorf("expected:\n%s\ngot:\n%s", want, got)
	}
}

func TestRemoveSkillSection_Chinese(t *testing.T) {
	input := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁\n\n# Skill 系统\n\n使用说明"
	got := removeSkillSection(input)

	want := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁"
	if got != want {
		t.Errorf("expected:\n%s\ngot:\n%s", want, got)
	}
}

func TestRemoveSkillSection_NoSkillMarker(t *testing.T) {
	input := "你是一个智能助手。\n\n## 工作原则\n1. 回复简洁"
	got := removeSkillSection(input)
	if got != input {
		t.Errorf("expected unchanged, got:\n%s", got)
	}
}

func TestRemoveSkillTool(t *testing.T) {
	tools := []tool.BaseTool{
		&mockSkillTool{name: "shell_execute", desc: "shell tool"},
		&mockSkillTool{name: "skill", desc: "skill tool"},
		&mockSkillTool{name: "transfer_to_agent", desc: "transfer tool"},
	}

	result := removeSkillTool(context.Background(), tools)
	if len(result) != 2 {
		t.Errorf("expected 2 tools after removal, got %d", len(result))
	}
	for _, t2 := range result {
		info, _ := t2.Info(context.Background())
		if info.Name == "skill" {
			t.Error("skill tool should have been removed")
		}
	}
}

func TestRemoveSkillTool_NotFound(t *testing.T) {
	tools := []tool.BaseTool{
		&mockSkillTool{name: "shell_execute", desc: "shell tool"},
		&mockSkillTool{name: "transfer_to_agent", desc: "transfer tool"},
	}

	result := removeSkillTool(context.Background(), tools)
	if len(result) != 2 {
		t.Errorf("expected 2 tools (unchanged), got %d", len(result))
	}
}

// --- Integration: BeforeAgent with runtime skill detection ---

func TestBeforeAgent_RemovesEmptySkills(t *testing.T) {
	f := &instructionFormatter{}
	ctx := context.Background()

	runCtx := &adk.ChatModelAgentContext{
		Instruction: "你是一个智能助手。\n\n# Skills System\n\n**How to Use Skills**\n\nSome content",
		Tools: []tool.BaseTool{
			&mockSkillTool{
				name: "skill",
				desc: `<available_skills></available_skills>`,
			},
		},
	}

	_, got, err := f.BeforeAgent(ctx, runCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if contains(got.Instruction, "Skills System") {
		t.Error("skill section should have been removed from instruction")
	}
	if contains(got.Instruction, "<skills_system>") {
		t.Error("<skills_system> tag should not be present")
	}
	if len(got.Tools) != 0 {
		t.Errorf("skill tool should have been removed, got %d tools", len(got.Tools))
	}
}

func TestBeforeAgent_KeepsSkillsWhenAvailable(t *testing.T) {
	f := &instructionFormatter{}
	ctx := context.Background()

	runCtx := &adk.ChatModelAgentContext{
		Instruction: "你是一个智能助手。\n\n# Skills System\n\n**How to Use Skills**\n\nSome content",
		Tools: []tool.BaseTool{
			&mockSkillTool{
				name: "skill",
				desc: `<available_skills>
<skill>
<name>
pdf
</name>
<description>
PDF tool
</description>
</skill>
</available_skills>`,
			},
		},
	}

	_, got, err := f.BeforeAgent(ctx, runCtx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !contains(got.Instruction, "<skills_system>") {
		t.Error("<skills_system> tag should be present when skills exist")
	}
	if len(got.Tools) != 1 {
		t.Errorf("skill tool should be kept, got %d tools", len(got.Tools))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
