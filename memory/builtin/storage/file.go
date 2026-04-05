package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/CoolBanHub/aggo/memory/builtin"
)

// FileStore 基于文件的记忆存储实现
// 分三个文件分别存储不同类型的数据，并在新增消息时支持修剪历史避免文件无限增长。
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
			if err := json.Unmarshal([]byte(line), &mem); err == nil {
				userMemories[mem.UserID] = &mem
			}
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
			if err := json.Unmarshal([]byte(line), &summary); err == nil {
				key := f.generateKey(summary.SessionID, summary.UserID)
				sessionSummaries[key] = &summary // 遇到同名的会被后续行覆盖（符合更新逻辑）
			}
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
			if err := json.Unmarshal([]byte(line), &msg); err == nil {
				key := f.generateKey(msg.SessionID, msg.UserID)
				messages[key] = append(messages[key], &msg)
			}
		}
		f.MemoryStore.messages = messages
	} else if !os.IsNotExist(err) {
		return err
	}

	return nil
}

// saveToFileB 原子的写入文件
func (f *FileStore) saveToFileB(name string, b []byte) error {
	filePath := f.getFilePath(name)
	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, filePath)
}

// saveUserMemories 重新全量保存用户记忆到 JSONL 文件
func (f *FileStore) saveUserMemories() error {
	f.MemoryStore.mu.RLock()
	var lines []string
	for _, mem := range f.MemoryStore.userMemories {
		if b, err := json.Marshal(mem); err == nil {
			lines = append(lines, string(b))
		}
	}
	f.MemoryStore.mu.RUnlock()

	data := []byte(strings.Join(lines, "\n") + "\n")
	if len(lines) == 0 {
		data = []byte("")
	}

	f.fileMu.Lock()
	defer f.fileMu.Unlock()
	return f.saveToFileB("user_memories", data)
}

// appendUserMemory 增量追加用户记忆到 JSONL 文件
func (f *FileStore) appendUserMemory(mem *builtin.UserMemory) error {
	b, err := json.Marshal(mem)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	f.fileMu.Lock()
	defer f.fileMu.Unlock()

	filePath := f.getFilePath("user_memories")
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(b)
	return err
}

// saveSessionSummaries 重新全量保存会话摘要到 JSONL 文件
func (f *FileStore) saveSessionSummaries() error {
	f.MemoryStore.mu.RLock()
	var lines []string
	for _, summary := range f.MemoryStore.sessionSummaries {
		if b, err := json.Marshal(summary); err == nil {
			lines = append(lines, string(b))
		}
	}
	f.MemoryStore.mu.RUnlock()

	data := []byte(strings.Join(lines, "\n") + "\n")
	if len(lines) == 0 {
		data = []byte("")
	}

	f.fileMu.Lock()
	defer f.fileMu.Unlock()
	return f.saveToFileB("session_summaries", data)
}

// appendSessionSummary 增量追加单条会话摘要到 JSONL 文件结尾
func (f *FileStore) appendSessionSummary(summary *builtin.SessionSummary) error {
	b, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	f.fileMu.Lock()
	defer f.fileMu.Unlock()

	filePath := f.getFilePath("session_summaries")
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(b)
	return err
}

// saveMessages 重新全量保存对话消息到 JSONL 文件
func (f *FileStore) saveMessages() error {
	f.MemoryStore.mu.RLock()
	var lines []string
	for _, sessionMsgs := range f.MemoryStore.messages {
		for _, msg := range sessionMsgs {
			if b, err := json.Marshal(msg); err == nil {
				lines = append(lines, string(b))
			}
		}
	}
	f.MemoryStore.mu.RUnlock()

	// 拼接所有行，加换行符
	data := []byte(strings.Join(lines, "\n") + "\n")
	if len(lines) == 0 {
		data = []byte("")
	}

	f.fileMu.Lock()
	defer f.fileMu.Unlock()
	return f.saveToFileB("messages", data)
}

// appendMessage 增量追加单条对话消息到 JSONL 文件结尾
func (f *FileStore) appendMessage(msg *builtin.ConversationMessage) error {
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	f.fileMu.Lock()
	defer f.fileMu.Unlock()

	filePath := f.getFilePath("messages")
	// 追加模式打开文件
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(b)
	return err
}

// === 下面覆写所有可能修改数据的方法，按对应的表分离落地存储 ===

// UpsertUserMemory 创建或更新用户记忆
// 由于每个用户只有一条记录，直接全量重写文件（记录数极少，开销可忽略）
func (f *FileStore) UpsertUserMemory(ctx context.Context, userMemory *builtin.UserMemory) error {
	if err := f.MemoryStore.UpsertUserMemory(ctx, userMemory); err != nil {
		return err
	}
	return f.saveUserMemories()
}

// ClearUserMemory 清空用户记忆
func (f *FileStore) ClearUserMemory(ctx context.Context, userID string) error {
	if err := f.MemoryStore.ClearUserMemory(ctx, userID); err != nil {
		return err
	}
	return f.saveUserMemories()
}

func (f *FileStore) SaveSessionSummary(ctx context.Context, summary *builtin.SessionSummary) error {
	if err := f.MemoryStore.SaveSessionSummary(ctx, summary); err != nil {
		return err
	}
	return f.appendSessionSummary(summary)
}

func (f *FileStore) UpdateSessionSummary(ctx context.Context, summary *builtin.SessionSummary) error {
	if err := f.MemoryStore.UpdateSessionSummary(ctx, summary); err != nil {
		return err
	}
	return f.appendSessionSummary(summary)
}

func (f *FileStore) DeleteSessionSummary(ctx context.Context, sessionID string, userID string) error {
	if err := f.MemoryStore.DeleteSessionSummary(ctx, sessionID, userID); err != nil {
		return err
	}
	return f.saveSessionSummaries()
}

func (f *FileStore) SaveMessage(ctx context.Context, message *builtin.ConversationMessage) error {
	if err := f.MemoryStore.SaveMessage(ctx, message); err != nil {
		return err
	}

	// 控制消息最大保存数量
	needRewrite := false
	if f.maxSessionMessages > 0 {
		count, _ := f.MemoryStore.GetMessageCount(ctx, message.UserID, message.SessionID)
		if count > f.maxSessionMessages {
			_ = f.MemoryStore.CleanupMessagesByLimit(ctx, message.UserID, message.SessionID, f.maxSessionMessages)
			needRewrite = true
		}
	}

	if needRewrite {
		return f.saveMessages() // 超过限额裁剪后，必须重写全量文件
	}
	return f.appendMessage(message) // 未超过额度时，直接以追加形式写入磁盘，O(1) 复杂度
}

func (f *FileStore) DeleteMessages(ctx context.Context, sessionID string, userID string) error {
	if err := f.MemoryStore.DeleteMessages(ctx, sessionID, userID); err != nil {
		return err
	}
	return f.saveMessages()
}

func (f *FileStore) CleanupOldMessages(ctx context.Context, userID string, before time.Time) error {
	if err := f.MemoryStore.CleanupOldMessages(ctx, userID, before); err != nil {
		return err
	}
	return f.saveMessages()
}

func (f *FileStore) CleanupMessagesByLimit(ctx context.Context, userID, sessionID string, keepLimit int) error {
	if err := f.MemoryStore.CleanupMessagesByLimit(ctx, userID, sessionID, keepLimit); err != nil {
		return err
	}
	return f.saveMessages()
}
