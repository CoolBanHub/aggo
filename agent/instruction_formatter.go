package agent

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// instructionFormatter is a post-processing middleware that wraps
// framework-injected skill sections with XML tags for better LLM comprehension.
//
// At runtime, it detects whether actual skills are available. If no skills
// exist, the entire # Skills System section and the skill tool are removed
// to avoid wasting tokens and confusing the LLM.
type instructionFormatter struct {
	*adk.TypedBaseChatModelAgentMiddleware[*schema.AgenticMessage]
}

func (f *instructionFormatter) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	instruction := runCtx.Instruction

	// Runtime check: if # Skills System section exists, verify actual skills are available.
	skillStart := findMarker(instruction,
		"# Skills System",
		"# Skill 系统",
	)

	if skillStart >= 0 && !hasAvailableSkills(ctx, runCtx.Tools) {
		// No skills available — strip the skills section from instruction
		instruction = removeSkillSection(instruction)
		// And remove the skill tool from the tool list
		runCtx.Tools = removeSkillTool(ctx, runCtx.Tools)
	}

	runCtx.Instruction = formatInstruction(instruction)
	return ctx, runCtx, nil
}

// hasAvailableSkills checks whether the skill tool has any actual skills
// by calling Info() at runtime (which triggers backend.List()).
func hasAvailableSkills(ctx context.Context, tools []tool.BaseTool) bool {
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		// The skill tool name defaults to "skill" but can be customized.
		// Check for the presence of <skill> entries in the description.
		if info.Name == "skill" {
			return strings.Contains(info.Desc, "<skill>\n<name>")
		}
	}
	return false
}

// removeSkillTool removes the skill tool from the tool list.
func removeSkillTool(ctx context.Context, tools []tool.BaseTool) []tool.BaseTool {
	for i, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name == "skill" {
			return append(tools[:i], tools[i+1:]...)
		}
	}
	return tools
}

// removeSkillSection strips everything from the # Skills System marker to
// the end of the instruction. The skill section is always appended last by
// the skill middleware, so this is safe.
func removeSkillSection(instruction string) string {
	skillStart := findMarker(instruction,
		"# Skills System",
		"# Skill 系统",
	)
	if skillStart < 0 {
		return instruction
	}
	return strings.TrimRight(instruction[:skillStart], " \t\n")
}

// formatInstruction detects framework-injected skill sections by their known
// markers and wraps them with XML tags.
//
// Before:
//
//	{base instruction}
//
//	# Skills System
//	...
//
// After:
//
//	{base instruction}
//
//	<skills_system>
//	# Skills System ...
//	</skills_system>
func formatInstruction(instruction string) string {
	skillStart := findMarker(instruction,
		"# Skills System",
		"# Skill 系统",
	)

	if skillStart < 0 {
		return instruction
	}

	var sb strings.Builder
	sb.WriteString(strings.TrimRight(instruction[:skillStart], " \t\n"))
	sb.WriteString("\n\n<skills_system>\n")
	sb.WriteString(strings.TrimSpace(instruction[skillStart:]))
	sb.WriteString("\n</skills_system>")

	return sb.String()
}

// findMarker returns the index of the first occurrence of any marker.
func findMarker(s string, markers ...string) int {
	first := -1
	for _, m := range markers {
		if idx := strings.Index(s, m); idx >= 0 {
			if first < 0 || idx < first {
				first = idx
			}
		}
	}
	return first
}
