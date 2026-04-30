package langfuse

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func cleanStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func mergeMetadata(values ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, value := range values {
		for k, v := range value {
			if strings.TrimSpace(k) != "" && v != nil {
				out[k] = v
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringMapAny(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if strings.TrimSpace(k) != "" {
			out[k] = v
		}
	}
	return out
}

func marshalAny(value any) any {
	if value == nil {
		return nil
	}
	return value
}

func safeJSON(value any) string {
	if value == nil {
		return ""
	}
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(b)
}

func modelParameters(conf *model.Config, toolChoice *schema.ToolChoice) map[string]any {
	params := map[string]any{}
	if conf != nil {
		if conf.Model != "" {
			params["model"] = conf.Model
		}
		if conf.MaxTokens > 0 {
			params["max_tokens"] = conf.MaxTokens
		}
		if conf.Temperature != 0 {
			params["temperature"] = conf.Temperature
		}
		if conf.TopP != 0 {
			params["top_p"] = conf.TopP
		}
		if len(conf.Stop) > 0 {
			params["stop"] = conf.Stop
		}
	}
	if toolChoice != nil {
		params["tool_choice"] = toolChoiceValue(toolChoice)
	}
	if len(params) == 0 {
		return nil
	}
	return params
}

func toolChoiceValue(toolChoice *schema.ToolChoice) any {
	if toolChoice == nil {
		return nil
	}
	switch *toolChoice {
	case schema.ToolChoiceForbidden:
		return "none"
	case schema.ToolChoiceAllowed:
		return "auto"
	case schema.ToolChoiceForced:
		return "required"
	default:
		return string(*toolChoice)
	}
}

func usageFromModel(usage *model.TokenUsage) (*usageBody, map[string]int) {
	if usage == nil {
		return nil, nil
	}
	body := &usageBody{
		Input:  usage.PromptTokens,
		Output: usage.CompletionTokens,
		Total:  usage.TotalTokens,
		Unit:   "TOKENS",
	}
	details := map[string]int{}
	if usage.PromptTokens > 0 {
		details["input"] = usage.PromptTokens
	}
	if usage.CompletionTokens > 0 {
		details["output"] = usage.CompletionTokens
	}
	if usage.TotalTokens > 0 {
		details["total"] = usage.TotalTokens
	}
	if usage.PromptTokenDetails.CachedTokens > 0 {
		details["input_cached"] = usage.PromptTokenDetails.CachedTokens
	}
	if usage.CompletionTokensDetails.ReasoningTokens > 0 {
		details["output_reasoning"] = usage.CompletionTokensDetails.ReasoningTokens
	}
	if len(details) == 0 {
		details = nil
	}
	return body, details
}

func getName(infoName, infoType, component string) string {
	if strings.TrimSpace(infoName) != "" {
		return strings.TrimSpace(infoName)
	}
	return strings.TrimSpace(infoType + component)
}
