package agentic

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

func AssistantMessage(text string) *schema.AgenticMessage {
	return &schema.AgenticMessage{
		Role: schema.AgenticRoleTypeAssistant,
		ContentBlocks: []*schema.ContentBlock{
			schema.NewContentBlock(&schema.AssistantGenText{Text: text}),
		},
	}
}

func Text(msg *schema.AgenticMessage) string {
	if msg == nil {
		return ""
	}
	parts := make([]string, 0, len(msg.ContentBlocks))
	for _, block := range msg.ContentBlocks {
		switch {
		case block == nil:
		case block.UserInputText != nil:
			parts = append(parts, block.UserInputText.Text)
		case block.AssistantGenText != nil:
			parts = append(parts, block.AssistantGenText.Text)
		case block.Reasoning != nil:
			parts = append(parts, block.Reasoning.Text)
		case block.FunctionToolResult != nil:
			parts = append(parts, functionToolResultText(block.FunctionToolResult))
		case block.ServerToolResult != nil:
			parts = append(parts, fmt.Sprint(block.ServerToolResult.Content))
		case block.MCPToolResult != nil:
			parts = append(parts, block.MCPToolResult.Content)
		}
	}
	return strings.Join(parts, "")
}

func Clone(msg *schema.AgenticMessage) *schema.AgenticMessage {
	if msg == nil {
		return nil
	}
	cloned := *msg
	if len(msg.ContentBlocks) > 0 {
		cloned.ContentBlocks = make([]*schema.ContentBlock, len(msg.ContentBlocks))
		for i, block := range msg.ContentBlocks {
			if block == nil {
				continue
			}
			blockCopy := *block
			if block.UserInputText != nil {
				text := *block.UserInputText
				blockCopy.UserInputText = &text
			}
			if block.AssistantGenText != nil {
				text := *block.AssistantGenText
				blockCopy.AssistantGenText = &text
			}
			cloned.ContentBlocks[i] = &blockCopy
		}
	}
	if msg.Extra != nil {
		cloned.Extra = make(map[string]any, len(msg.Extra))
		for k, v := range msg.Extra {
			cloned.Extra[k] = v
		}
	}
	return &cloned
}

func PrependText(msg *schema.AgenticMessage, prefix string) *schema.AgenticMessage {
	if msg == nil {
		return schema.UserAgenticMessage(prefix)
	}
	cloned := Clone(msg)
	for _, block := range cloned.ContentBlocks {
		if block == nil {
			continue
		}
		if block.UserInputText != nil {
			block.UserInputText.Text = prefix + block.UserInputText.Text
			return cloned
		}
		if block.AssistantGenText != nil {
			block.AssistantGenText.Text = prefix + block.AssistantGenText.Text
			return cloned
		}
	}
	cloned.ContentBlocks = append([]*schema.ContentBlock{textBlockForRole(cloned.Role, prefix)}, cloned.ContentBlocks...)
	return cloned
}

func functionToolResultText(result *schema.FunctionToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	parts := make([]string, 0, len(result.Content))
	for _, block := range result.Content {
		if block == nil {
			continue
		}
		if block.Text != nil {
			parts = append(parts, block.Text.Text)
			continue
		}
		parts = append(parts, strings.TrimSpace(block.String()))
	}
	return strings.Join(parts, "")
}

func HasFunctionToolCall(msg *schema.AgenticMessage) bool {
	if msg == nil {
		return false
	}
	for _, block := range msg.ContentBlocks {
		if block != nil && block.FunctionToolCall != nil {
			return true
		}
	}
	return false
}

func InputParts(msg *schema.AgenticMessage) []schema.MessageInputPart {
	if msg == nil {
		return nil
	}
	parts := make([]schema.MessageInputPart, 0, len(msg.ContentBlocks))
	for _, block := range msg.ContentBlocks {
		if block == nil {
			continue
		}
		switch {
		case block.UserInputText != nil:
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeText,
				Text: block.UserInputText.Text,
			})
		case block.UserInputImage != nil:
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeImageURL,
				Image: &schema.MessageInputImage{
					MessagePartCommon: schema.MessagePartCommon{
						URL:        ptrOrNil(block.UserInputImage.URL),
						Base64Data: ptrOrNil(block.UserInputImage.Base64Data),
						MIMEType:   block.UserInputImage.MIMEType,
					},
					Detail: block.UserInputImage.Detail,
				},
			})
		case block.UserInputAudio != nil:
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeAudioURL,
				Audio: &schema.MessageInputAudio{MessagePartCommon: schema.MessagePartCommon{
					URL:        ptrOrNil(block.UserInputAudio.URL),
					Base64Data: ptrOrNil(block.UserInputAudio.Base64Data),
					MIMEType:   block.UserInputAudio.MIMEType,
				}},
			})
		case block.UserInputVideo != nil:
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeVideoURL,
				Video: &schema.MessageInputVideo{MessagePartCommon: schema.MessagePartCommon{
					URL:        ptrOrNil(block.UserInputVideo.URL),
					Base64Data: ptrOrNil(block.UserInputVideo.Base64Data),
					MIMEType:   block.UserInputVideo.MIMEType,
				}},
			})
		case block.UserInputFile != nil:
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeFileURL,
				File: &schema.MessageInputFile{
					MessagePartCommon: schema.MessagePartCommon{
						URL:        ptrOrNil(block.UserInputFile.URL),
						Base64Data: ptrOrNil(block.UserInputFile.Base64Data),
						MIMEType:   block.UserInputFile.MIMEType,
					},
					Name: block.UserInputFile.Name,
				},
			})
		}
	}
	return parts
}

func UserMessageFromInputParts(parts []schema.MessageInputPart) *schema.AgenticMessage {
	msg := &schema.AgenticMessage{Role: schema.AgenticRoleTypeUser}
	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			msg.ContentBlocks = append(msg.ContentBlocks, schema.NewContentBlock(&schema.UserInputText{Text: part.Text}))
		case schema.ChatMessagePartTypeImageURL:
			if part.Image != nil {
				msg.ContentBlocks = append(msg.ContentBlocks, schema.NewContentBlock(&schema.UserInputImage{
					URL:        valueOrEmpty(part.Image.URL),
					Base64Data: valueOrEmpty(part.Image.Base64Data),
					MIMEType:   part.Image.MIMEType,
					Detail:     part.Image.Detail,
				}))
			}
		case schema.ChatMessagePartTypeAudioURL:
			if part.Audio != nil {
				msg.ContentBlocks = append(msg.ContentBlocks, schema.NewContentBlock(&schema.UserInputAudio{
					URL:        valueOrEmpty(part.Audio.URL),
					Base64Data: valueOrEmpty(part.Audio.Base64Data),
					MIMEType:   part.Audio.MIMEType,
				}))
			}
		case schema.ChatMessagePartTypeVideoURL:
			if part.Video != nil {
				msg.ContentBlocks = append(msg.ContentBlocks, schema.NewContentBlock(&schema.UserInputVideo{
					URL:        valueOrEmpty(part.Video.URL),
					Base64Data: valueOrEmpty(part.Video.Base64Data),
					MIMEType:   part.Video.MIMEType,
				}))
			}
		case schema.ChatMessagePartTypeFileURL:
			if part.File != nil {
				msg.ContentBlocks = append(msg.ContentBlocks, schema.NewContentBlock(&schema.UserInputFile{
					URL:        valueOrEmpty(part.File.URL),
					Name:       part.File.Name,
					Base64Data: valueOrEmpty(part.File.Base64Data),
					MIMEType:   part.File.MIMEType,
				}))
			}
		}
	}
	return msg
}

func ptrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func AppendUserText(msg *schema.AgenticMessage, text string) *schema.AgenticMessage {
	if msg == nil {
		return schema.UserAgenticMessage(text)
	}
	msg.ContentBlocks = append(msg.ContentBlocks, textBlockForRole(msg.Role, text))
	return msg
}

func textBlockForRole(role schema.AgenticRoleType, text string) *schema.ContentBlock {
	if role == schema.AgenticRoleTypeAssistant {
		return schema.NewContentBlock(&schema.AssistantGenText{Text: text})
	}
	return schema.NewContentBlock(&schema.UserInputText{Text: text})
}
