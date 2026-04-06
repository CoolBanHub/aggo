package agent

import (
	"testing"
)

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
