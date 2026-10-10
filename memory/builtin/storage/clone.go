package storage

import (
	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/cloudwego/eino/schema"
)

// 存储层对象以不可变快照方式出入库：保存时深拷贝入参，读取时深拷贝内部对象。
// 否则调用方在存储锁外修改共享指针（如 Parts 里的 URL、事件 Keywords）
// 会绕过存储锁直接改变存储内容（改进方案 §5.4）。

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	cloned := *v
	return &cloned
}

func cloneUserMemory(src *builtin.UserMemory) *builtin.UserMemory {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func cloneSessionSummary(src *builtin.SessionSummary) *builtin.SessionSummary {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func cloneConversationMessage(src *builtin.ConversationMessage) *builtin.ConversationMessage {
	if src == nil {
		return nil
	}
	cloned := *src
	cloned.Parts = cloneMessageInputParts(src.Parts)
	return &cloned
}

func cloneUserMemoryEvent(src *builtin.UserMemoryEvent) *builtin.UserMemoryEvent {
	if src == nil {
		return nil
	}
	cloned := *src
	if len(src.Keywords) > 0 {
		cloned.Keywords = append([]string(nil), src.Keywords...)
	}
	return &cloned
}

func cloneMessageInputParts(parts []schema.MessageInputPart) []schema.MessageInputPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]schema.MessageInputPart, len(parts))
	for i, part := range parts {
		cloned[i] = part
		if part.Image != nil {
			image := *part.Image
			image.MessagePartCommon = clonePartCommon(part.Image.MessagePartCommon)
			cloned[i].Image = &image
		}
		if part.Audio != nil {
			audio := *part.Audio
			audio.MessagePartCommon = clonePartCommon(part.Audio.MessagePartCommon)
			cloned[i].Audio = &audio
		}
		if part.Video != nil {
			video := *part.Video
			video.MessagePartCommon = clonePartCommon(part.Video.MessagePartCommon)
			cloned[i].Video = &video
		}
		if part.File != nil {
			file := *part.File
			file.MessagePartCommon = clonePartCommon(part.File.MessagePartCommon)
			cloned[i].File = &file
		}
	}
	return cloned
}

func clonePartCommon(src schema.MessagePartCommon) schema.MessagePartCommon {
	cloned := src
	cloned.URL = cloneStringPtr(src.URL)
	cloned.Base64Data = cloneStringPtr(src.Base64Data)
	return cloned
}
