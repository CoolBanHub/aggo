package agent

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

// instructionFormatter is a post-processing middleware that wraps
// framework-injected instruction sections (sub-agent transfer, skills)
// with XML tags for better LLM comprehension.
//
// At runtime, it detects whether actual skills are available. If no skills
// exist, the entire # Skills System section and the skill tool are removed
// to avoid wasting tokens and confusing the LLM.
type instructionFormatter struct {
	*adk.BaseChatModelAgentMiddleware
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

// formatInstruction detects framework-injected sections by their known markers
// and wraps each section with XML tags.
//
// Before:
//
//	{base instruction}
//
// Available other agents:
//
//   - Agent name: cron
//     Agent description: ...
//     Decision rule: ...
//
//     # Skills System
//     ...
//
// After:
//
//	{base instruction}
//
//	<available_agents>
//	Available other agents:
//	- Agent name: cron ...
//	</available_agents>
//
//	<skills_system>
//	# Skills System ...
//	</skills_system>
func formatInstruction(instruction string) string {
	transferStart := findMarker(instruction,
		"Available other agents:",
		"可用的其他 agent",
	)
	skillStart := findMarker(instruction,
		"# Skills System",
		"# Skill 系统",
	)

	if transferStart < 0 && skillStart < 0 {
		return instruction
	}

	// Determine section boundaries
	type section struct {
		start int
		end   int
		tag   string
	}

	var sections []section

	if transferStart >= 0 {
		end := len(instruction)
		if skillStart > transferStart {
			end = skipLeadingNewlines(instruction, skillStart)
		}
		sections = append(sections, section{start: transferStart, end: end, tag: "available_agents"})
	}

	if skillStart >= 0 {
		sections = append(sections, section{
			start: skillStart,
			end:   len(instruction),
			tag:   "skills_system",
		})
	}

	// Build result
	var sb strings.Builder
	baseEnd := sections[0].start
	sb.WriteString(strings.TrimRight(instruction[:baseEnd], " \t\n"))

	for _, sec := range sections {
		content := strings.TrimSpace(instruction[sec.start:sec.end])
		sb.WriteString("\n\n<")
		sb.WriteString(sec.tag)
		sb.WriteString(">\n")
		sb.WriteString(content)
		sb.WriteString("\n</")
		sb.WriteString(sec.tag)
		sb.WriteString(">")
	}

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

// skipLeadingNewlines skips any leading newlines before the given position.
func skipLeadingNewlines(s string, pos int) int {
	for pos > 0 && (s[pos-1] == '\n' || s[pos-1] == '\r') {
		pos--
	}
	return pos
}
