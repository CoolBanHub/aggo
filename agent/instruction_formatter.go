package agent

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/adk"
)

// instructionFormatter is a post-processing middleware that wraps
// framework-injected instruction sections (sub-agent transfer, skills)
// with XML tags for better LLM comprehension.
type instructionFormatter struct {
	*adk.BaseChatModelAgentMiddleware
}

func (f *instructionFormatter) BeforeAgent(ctx context.Context, runCtx *adk.ChatModelAgentContext) (context.Context, *adk.ChatModelAgentContext, error) {
	runCtx.Instruction = formatInstruction(runCtx.Instruction)
	return ctx, runCtx, nil
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
