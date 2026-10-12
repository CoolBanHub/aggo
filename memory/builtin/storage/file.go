package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CoolBanHub/aggo/memory/builtin"
)

// FileStore 基于文件的记忆存储实现
// 按数据类型分文件保存。写入使用完整候选快照与原子替换，
// 消息文件较大或写入频繁时建议使用 SQL 存储。
type FileStore struct {
	*MemoryStore
	dirPath            string
	maxSessionMessages int
	fileMu             sync.Mutex // 防止并发写文件
}

// NewFileStore 创建新的基于文件的存储实例
// dirPath: 保存数据的目录路径
// maxSessionMessages: 每个会话最大保存的消息数量，超过会裁剪旧消息。如果传 <= 0，默认保留100条。
func NewFileStore(dirPath string, maxSessionMessages int) (*FileStore, error) {
	if maxSessionMessages <= 0 {
		maxSessionMessages = 300
	}

	// 确保目录存在
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, err
	}

	fs := &FileStore{
		MemoryStore:        NewMemoryStore(),
		dirPath:            dirPath,
		maxSessionMessages: maxSessionMessages,
	}

	if err := fs.load(); err != nil {
		return nil, err
	}

	return fs, nil
}

// getFilePath 获取配置文件完整路径
func (f *FileStore) getFilePath(name string) string {
	return filepath.Join(f.dirPath, name+".json")
}

// load 从拆分的三个文件加载数据
func (f *FileStore) load() error {
	f.fileMu.Lock()
	defer f.fileMu.Unlock()

	f.MemoryStore.mu.Lock()
	defer f.MemoryStore.mu.Unlock()

	// 1. 加载用户记忆 (JSONL 格式，每用户一条记录)
	if data, err := os.ReadFile(f.getFilePath("user_memories")); err == nil {
		userMemories := make(map[string]*builtin.UserMemory)
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var mem builtin.UserMemory
			if err := json.Unmarshal([]byte(line), &mem); err != nil {
				return fmt.Errorf("decode user memory: %w", err)
			}
			userMemories[mem.UserID] = &mem
		}
		f.MemoryStore.userMemories = userMemories
	} else if !os.IsNotExist(err) {
		return err
	}

	// 2. 加载会话摘要 (JSONL 格式)
	if data, err := os.ReadFile(f.getFilePath("session_summaries")); err == nil {
		sessionSummaries := make(map[string]*builtin.SessionSummary)
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var summary builtin.SessionSummary
			if err := json.Unmarshal([]byte(line), &summary); err != nil {
				return fmt.Errorf("decode session summary: %w", err)
			}
			key := f.generateKey(summary.SessionID, summary.UserID)
			sessionSummaries[key] = &summary // 遇到同名的会被后续行覆盖（符合更新逻辑）
		}
		f.MemoryStore.sessionSummaries = sessionSummaries
	} else if !os.IsNotExist(err) {
		return err
	}

	// 3. 加载对话消息 (JSONL 格式)
	if data, err := os.ReadFile(f.getFilePath("messages")); err == nil {
		messages := make(map[string][]*builtin.ConversationMessage)
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var msg builtin.ConversationMessage
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				return fmt.Errorf("decode conversation message: %w", err)
			}
			key := f.generateKey(msg.SessionID, msg.UserID)
			messages[key] = append(messages[key], &msg)
		}
		f.MemoryStore.messages = messages
	} else if !os.IsNotExist(err) {
		return err
	}

	// 4. 加载用户记忆事件 (JSONL 格式)
	if data, err := os.ReadFile(f.getFilePath("user_memory_events")); err == nil {
		events := make(map[string][]*builtin.UserMemoryEvent)
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var evt builtin.UserMemoryEvent
			if err := json.Unmarshal([]byte(line), &evt); err != nil {
				return fmt.Errorf("decode user memory event: %w", err)
			}
			events[evt.UserID] = append(events[evt.UserID], &evt)
		}
		f.MemoryStore.userMemoryEvents = events
	} else if !os.IsNotExist(err) {
		return err
	}

	return nil
}

// mutate serializes the entire mutation, persistence and publication. Readers
// keep seeing the committed snapshot until Rename succeeds. A failed write
// leaves both the in-memory store and its previous file intact.
func (f *FileStore) mutate(ctx context.Context, name string, change func(*MemoryStore) error) error {
	f.fileMu.Lock()
	defer f.fileMu.Unlock()
	f.MemoryStore.mu.Lock()
	defer f.MemoryStore.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	candidate := NewMemoryStore()
	// Clone only the affected table; unrelated committed maps are untouched.
	candidate.userMemories = f.userMemories
	candidate.sessionSummaries = f.sessionSummaries
	candidate.messages = f.messages
	candidate.userMemoryEvents = f.userMemoryEvents
	switch name {
	case "user_memories":
		candidate.userMemories = make(map[string]*builtin.UserMemory, len(f.userMemories))
		for k, v := range f.userMemories {
			candidate.userMemories[k] = cloneUserMemory(v)
		}
	case "session_summaries":
		candidate.sessionSummaries = make(map[string]*builtin.SessionSummary, len(f.sessionSummaries))
		for k, v := range f.sessionSummaries {
			candidate.sessionSummaries[k] = cloneSessionSummary(v)
		}
	case "messages":
		candidate.messages = make(map[string][]*builtin.ConversationMessage, len(f.messages))
		for k, list := range f.messages {
			for _, v := range list {
				candidate.messages[k] = append(candidate.messages[k], cloneConversationMessage(v))
			}
		}
	case "user_memory_events":
		candidate.userMemoryEvents = make(map[string][]*builtin.UserMemoryEvent, len(f.userMemoryEvents))
		for k, list := range f.userMemoryEvents {
			candidate.userMemoryEvents[k] = cloneEventSlice(list)
		}
	default:
		return fmt.Errorf("unknown memory file: %s", name)
	}
	if err := change(candidate); err != nil {
		return err
	}
	var records []any
	switch name {
	case "user_memories":
		for _, v := range candidate.userMemories {
			records = append(records, v)
		}
	case "session_summaries":
		for _, v := range candidate.sessionSummaries {
			records = append(records, v)
		}
	case "messages":
		for _, list := range candidate.messages {
			for _, v := range list {
				records = append(records, v)
			}
		}
	case "user_memory_events":
		for _, list := range candidate.userMemoryEvents {
			for _, v := range list {
				records = append(records, v)
			}
		}
	default:
		return fmt.Errorf("unknown memory file: %s", name)
	}
	var data strings.Builder
	encoder := json.NewEncoder(&data)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	if err := f.saveToFileB(name, []byte(data.String())); err != nil {
		return err
	}
	f.userMemories = candidate.userMemories
	f.sessionSummaries = candidate.sessionSummaries
	f.messages = candidate.messages
	f.userMemoryEvents = candidate.userMemoryEvents
	return nil
}

func (f *FileStore) saveToFileB(name string, data []byte) error {
	path := f.getFilePath(name)
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}
	if dir, err := os.Open(f.dirPath); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (f *FileStore) UpsertUserMemory(ctx context.Context, v *builtin.UserMemory) error {
	return f.mutate(ctx, "user_memories", func(s *MemoryStore) error { return s.UpsertUserMemory(ctx, v) })
}
func (f *FileStore) ClearUserMemory(ctx context.Context, userID string) error {
	return f.mutate(ctx, "user_memories", func(s *MemoryStore) error { return s.ClearUserMemory(ctx, userID) })
}
func (f *FileStore) SaveSessionSummary(ctx context.Context, v *builtin.SessionSummary) error {
	return f.mutate(ctx, "session_summaries", func(s *MemoryStore) error { return s.SaveSessionSummary(ctx, v) })
}
func (f *FileStore) UpdateSessionSummary(ctx context.Context, v *builtin.SessionSummary) error {
	return f.mutate(ctx, "session_summaries", func(s *MemoryStore) error { return s.UpdateSessionSummary(ctx, v) })
}
func (f *FileStore) DeleteSessionSummary(ctx context.Context, sessionID, userID string) error {
	return f.mutate(ctx, "session_summaries", func(s *MemoryStore) error { return s.DeleteSessionSummary(ctx, sessionID, userID) })
}
func (f *FileStore) SaveMessage(ctx context.Context, v *builtin.ConversationMessage) error {
	return f.mutate(ctx, "messages", func(s *MemoryStore) error {
		if err := s.SaveMessage(ctx, v); err != nil {
			return err
		}
		return s.CleanupMessagesByLimit(ctx, v.UserID, v.SessionID, f.maxSessionMessages)
	})
}
func (f *FileStore) DeleteMessages(ctx context.Context, sessionID, userID string) error {
	return f.mutate(ctx, "messages", func(s *MemoryStore) error { return s.DeleteMessages(ctx, sessionID, userID) })
}
func (f *FileStore) CleanupOldMessages(ctx context.Context, userID string, before time.Time) error {
	return f.mutate(ctx, "messages", func(s *MemoryStore) error { return s.CleanupOldMessages(ctx, userID, before) })
}
func (f *FileStore) CleanupMessagesByLimit(ctx context.Context, userID, sessionID string, limit int) error {
	return f.mutate(ctx, "messages", func(s *MemoryStore) error { return s.CleanupMessagesByLimit(ctx, userID, sessionID, limit) })
}
func (f *FileStore) SaveUserMemoryEvent(ctx context.Context, v *builtin.UserMemoryEvent) error {
	return f.mutate(ctx, "user_memory_events", func(s *MemoryStore) error { return s.SaveUserMemoryEvent(ctx, v) })
}
func (f *FileStore) DeleteUserMemoryEvent(ctx context.Context, userID, eventID string) error {
	return f.mutate(ctx, "user_memory_events", func(s *MemoryStore) error { return s.DeleteUserMemoryEvent(ctx, userID, eventID) })
}
func (f *FileStore) ClearUserMemoryEvents(ctx context.Context, userID string) error {
	return f.mutate(ctx, "user_memory_events", func(s *MemoryStore) error { return s.ClearUserMemoryEvents(ctx, userID) })
}
