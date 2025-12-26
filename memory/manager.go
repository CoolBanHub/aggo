package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/gookit/slog"
)

// MemoryManager 记忆管理器
// 负责管理用户记忆、会话摘要和对话历史
type MemoryManager struct {
	// 存储接口
	storage MemoryStorage
	// 记忆配置
	config *MemoryConfig

	userMemoryAnalyzer      *UserMemoryAnalyzer
	sessionSummaryGenerator *SessionSummaryGenerator

	// 摘要触发管理
	summaryTrigger *SummaryTriggerManager

	// 异步处理相关
	taskChannel chan asyncTask
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc

	// 定期清理相关
	cleanupTicker *time.Ticker
	cleanupWg     sync.WaitGroup
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc

	// 异步任务队列统计
	taskQueueStats TaskQueueStats
	taskQueueMutex sync.RWMutex
}

// asyncTask 异步任务结构
type asyncTask struct {
	taskType  string // "memory" 或 "summary"
	userID    string
	sessionID string
	message   string
	parts     []schema.MessageInputPart
}

/**
  📊 默认清理策略

  - 会话状态: 保留7天，每12小时清理一次
  - 对话消息: 单会话最多1000条，保留30天
  - 异步队列: 10倍工作池大小的缓冲区
  - 定期清理: 每12小时执行一次
*/

// NewMemoryManager 创建新的记忆管理器
func NewMemoryManager(cm model.ToolCallingChatModel, memoryStorage MemoryStorage, config *MemoryConfig) (*MemoryManager, error) {
	if config == nil {
		config = &MemoryConfig{
			EnableUserMemories:   true,
			EnableSessionSummary: false,
			Retrieval:            RetrievalLastN,
			MemoryLimit:          20,
			AsyncProcessing:      true,
			AsyncWorkerPoolSize:  5,
			SummaryTrigger: SummaryTriggerConfig{
				Strategy:         TriggerSmart,
				MessageThreshold: 10,  // MemoryLimit的一半
				MinInterval:      600, // 600秒最小间隔
			},
			// 默认清理配置
			SessionCleanupInterval: 24,   // 24小时清理一次会话状态
			SessionRetentionTime:   168,  // 7天保留时间
			MessageHistoryLimit:    1000, // 1000条消息限制
			MessageRetentionTime:   720,  // 30天消息保留时间
			CleanupInterval:        12,   // 12小时定期清理
		}
	}

	// 设置表前缀
	if config.TablePre != "" {
		memoryStorage.SetTablePrefix(config.TablePre)
	}

	err := memoryStorage.AutoMigrate()
	if err != nil {
		return nil, err
	}

	if config.MemoryLimit == 0 {
		config.MemoryLimit = 30
	}

	if config.EnableSessionSummary && config.SummaryTrigger.MessageThreshold == 0 && !(config.SummaryTrigger.Strategy == TriggerSmart || config.SummaryTrigger.Strategy == TriggerByMessages) {
		config.SummaryTrigger.MessageThreshold = config.MemoryLimit / 2
	}

	// 设置异步处理的默认值
	if config.AsyncWorkerPoolSize <= 0 {
		config.AsyncWorkerPoolSize = 5
	}

	ctx, cancel := context.WithCancel(context.Background())
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())

	manager := &MemoryManager{
		storage:                 memoryStorage,
		config:                  config,
		userMemoryAnalyzer:      NewUserMemoryAnalyzer(cm),
		sessionSummaryGenerator: NewSessionSummaryGenerator(cm),
		summaryTrigger:          NewSummaryTriggerManager(config.SummaryTrigger),
		ctx:                     ctx,
		cancel:                  cancel,
		cleanupCtx:              cleanupCtx,
		cleanupCancel:           cleanupCancel,
	}

	// 如果启用异步处理，初始化goroutine池
	if config.AsyncProcessing {
		// 增大队列缓冲区，减少任务丢失的可能性
		queueCapacity := config.AsyncWorkerPoolSize * 10 // 缓冲区大小为工作池的10倍
		manager.taskChannel = make(chan asyncTask, queueCapacity)
		manager.taskQueueStats.QueueCapacity = queueCapacity
		manager.startAsyncWorkers()
	}

	// 启动定期清理任务
	manager.startPeriodicCleanup()

	return manager, nil
}

// startAsyncWorkers 启动异步工作goroutine池
func (m *MemoryManager) startAsyncWorkers() {
	for i := 0; i < m.config.AsyncWorkerPoolSize; i++ {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			for {
				select {
				case <-m.ctx.Done():
					return
				case task := <-m.taskChannel:
					m.updateQueueStats(-1) // 减少队列大小
					m.processAsyncTask(task)
					atomic.AddInt64(&m.taskQueueStats.ProcessedTasks, 1)
				}
			}
		}()
	}
	m.taskQueueStats.ActiveWorkers = m.config.AsyncWorkerPoolSize
}

// updateQueueStats 更新队列统计
func (m *MemoryManager) updateQueueStats(delta int) {
	m.taskQueueMutex.Lock()
	defer m.taskQueueMutex.Unlock()

	m.taskQueueStats.QueueSize += delta
	if m.taskQueueStats.QueueCapacity > 0 {
		m.taskQueueStats.QueueUtilization = float64(m.taskQueueStats.QueueSize) / float64(m.taskQueueStats.QueueCapacity)
	}
}

// submitAsyncTask 提交异步任务，改进错误处理
func (m *MemoryManager) submitAsyncTask(task asyncTask) bool {
	if !m.config.AsyncProcessing {
		return false
	}

	select {
	case m.taskChannel <- task:
		m.updateQueueStats(1) // 增加队列大小
		return true
	default:
		// 队列满，增加丢弃计数
		atomic.AddInt64(&m.taskQueueStats.DroppedTasks, 1)

		// 记录详细统计信息
		m.taskQueueMutex.RLock()
		queueSize := m.taskQueueStats.QueueSize
		capacity := m.taskQueueStats.QueueCapacity
		dropped := m.taskQueueStats.DroppedTasks
		m.taskQueueMutex.RUnlock()

		slog.Errorf("异步任务队列已满，丢弃任务. 队列: %d/%d, 总丢弃: %d, 任务类型: %s, 用户: %s",
			queueSize, capacity, dropped, task.taskType, task.userID)
		return false
	}
}

// GetTaskQueueStats 获取异步任务队列统计
func (m *MemoryManager) GetTaskQueueStats() TaskQueueStats {
	m.taskQueueMutex.RLock()
	defer m.taskQueueMutex.RUnlock()

	// 更新当前队列大小
	if m.taskChannel != nil {
		m.taskQueueStats.QueueSize = len(m.taskChannel)
		if m.taskQueueStats.QueueCapacity > 0 {
			m.taskQueueStats.QueueUtilization = float64(m.taskQueueStats.QueueSize) / float64(m.taskQueueStats.QueueCapacity)
		}
	}

	return m.taskQueueStats
}

// startPeriodicCleanup 启动定期清理任务
func (m *MemoryManager) startPeriodicCleanup() {
	if m.config.CleanupInterval <= 0 {
		m.config.CleanupInterval = 12 // 默认12小时
	}

	m.cleanupTicker = time.NewTicker(time.Duration(m.config.CleanupInterval) * time.Hour)
	m.cleanupWg.Add(1)
	go func() {
		defer m.cleanupWg.Done()
		for {
			select {
			case <-m.cleanupCtx.Done():
				m.cleanupTicker.Stop()
				return
			case <-m.cleanupTicker.C:
				m.performPeriodicCleanup()
			}
		}
	}()
}

// performPeriodicCleanup 执行定期清理
func (m *MemoryManager) performPeriodicCleanup() {
	// 创建超时context，避免清理任务阻塞
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 1. 清理旧的会话状态
	if m.config.SessionCleanupInterval > 0 {
		sessionRetention := time.Duration(m.config.SessionRetentionTime) * time.Hour
		if sessionRetention <= 0 {
			sessionRetention = 168 * time.Hour // 默认7天
		}
		m.summaryTrigger.CleanupOldSessions(sessionRetention)
		slog.Infof("定期清理: 清理了 %v 小时前的会话状态", sessionRetention.Hours())
	}

	// 2. 清理旧的消息历史（按时间）
	if m.config.MessageRetentionTime > 0 {
		messageRetention := time.Duration(m.config.MessageRetentionTime) * time.Hour
		cutoff := time.Now().Add(-messageRetention)

		// 这里可以添加按用户清理的逻辑，需要获取所有用户列表
		// 目前只记录执行日志，具体清理由各存储实现处理
		slog.Infof("定期清理: 清理 %v 之前的消息历史", cutoff.Format("2006-01-02 15:04:05"))

		// 示例：清理管理员用户的历史消息（实际应用中需要遍历所有活跃用户）
		err := m.storage.CleanupOldMessages(ctx, "admin", cutoff)
		if err != nil {
			slog.Errorf("清理旧消息失败: %v", err)
		}
	}

	// 3. 按数量限制清理消息
	if m.config.MessageHistoryLimit > 0 {
		// 这里需要获取所有活跃用户的会话，然后逐个清理
		// 由于存储接口限制，暂时只记录日志
		slog.Infof("定期清理: 消息历史限制设置为 %d 条", m.config.MessageHistoryLimit)
	}
}

// processAsyncTask 处理异步任务
func (m *MemoryManager) processAsyncTask(task asyncTask) {
	switch task.taskType {
	case "memory":
		// 创建新的context用于异步操作，避免超时
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // 30秒超时
		defer cancel()
		m.analyzeAndCreateUserMemory(ctx, task.userID, task.message, task.parts)
	case "summary":
		// 创建新的context用于异步操作
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // 30秒超时
		defer cancel()
		err := m.updateSessionSummary(ctx, task.userID, task.sessionID)
		if err != nil {
			slog.Errorf("异步更新会话摘要失败: sessionID=%s, userID=%s, err=%v\n", task.sessionID, task.userID, err)
		} else {
			// 标记摘要已更新
			m.summaryTrigger.MarkSummaryUpdated(generateSessionKey(task.userID, task.sessionID))
		}
	}
}

// ProcessUserMessage 处理包含多部分内容的用户消息
// 根据配置决定是否创建用户记忆、更新会话摘要等
// messageID: 可选的消息ID，如果为空则自动生成
func (m *MemoryManager) ProcessUserMessage(ctx context.Context, userID, sessionID, messageID, content string, parts []schema.MessageInputPart) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if content == "" && len(parts) == 0 {
		return errors.New("用户消息内容不能为空")
	}

	// 检查消息数量并可能清理旧消息
	if m.config.MessageHistoryLimit > 0 {
		currentCount, err := m.storage.GetMessageCount(ctx, userID, sessionID)
		if err != nil {
			slog.Errorf("获取消息数量失败: %v", err)
		} else if currentCount >= m.config.MessageHistoryLimit {
			// 清理超出限制的消息，保留最新的N条
			err := m.storage.CleanupMessagesByLimit(ctx, userID, sessionID, m.config.MessageHistoryLimit-1)
			if err != nil {
				slog.Errorf("清理超限消息失败: %v", err)
			} else {
				slog.Infof("会话 %s 消息数量达到限制 %d，已清理旧消息", sessionID, m.config.MessageHistoryLimit)
			}
		}
	}

	// 保存用户消息到对话历史
	err := m.SaveMessage(ctx, &ConversationMessage{
		ID:        messageID,
		SessionID: sessionID,
		UserID:    userID,
		Role:      "user",
		Content:   content,
		Parts:     parts,
	})
	if err != nil {
		return fmt.Errorf("保存用户消息失败: %v", err)
	}

	// 如果启用了用户记忆，分析消息并创建记忆
	if m.config.EnableUserMemories {
		if m.config.AsyncProcessing {
			// 异步处理，使用改进的任务提交方法
			submitted := m.submitAsyncTask(asyncTask{
				taskType: "memory",
				userID:   userID,
				message:  content,
				parts:    parts,
			})
			if !submitted {
				slog.Errorf("警告: 用户记忆分析队列已满，跳过处理: userID=%s\n", userID)
			}
		} else {
			// 同步处理
			m.analyzeAndCreateUserMemory(ctx, userID, content, parts)
		}
	}

	return nil
}

// ProcessAssistantMessage 处理助手回复消息
// messageID: 可选的消息ID，如果为空则自动生成
func (m *MemoryManager) ProcessAssistantMessage(ctx context.Context, userID, sessionID, messageID, assistantMessage string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if assistantMessage == "" {
		return errors.New("助手消息不能为空")
	}

	// 保存助手消息到对话历史
	err := m.SaveMessage(ctx, &ConversationMessage{
		ID:        messageID,
		SessionID: sessionID,
		UserID:    userID,
		Role:      string(schema.Assistant),
		Content:   assistantMessage,
	})
	if err != nil {
		return fmt.Errorf("保存助手消息失败: %v", err)
	}

	// 如果启用了会话摘要，检查是否需要更新摘要
	if m.config.EnableSessionSummary {
		shouldTrigger, err := m.shouldTriggerSummaryUpdate(ctx, userID, sessionID)
		if err != nil {
			slog.Errorf("检查摘要触发条件失败: %v\n", err)
		} else if shouldTrigger {
			if m.config.AsyncProcessing {
				// 异步处理，使用改进的任务提交方法
				submitted := m.submitAsyncTask(asyncTask{
					taskType:  "summary",
					userID:    userID,
					sessionID: sessionID,
				})
				if !submitted {
					slog.Errorf("警告: 会话摘要更新队列已满，跳过处理: sessionID=%s, userID=%s\n", sessionID, userID)
				}
			} else {
				// 同步处理
				err = m.updateSessionSummary(ctx, userID, sessionID)
				if err != nil {
					slog.Errorf("更新会话摘要失败: msg:%s,err:%v\n", assistantMessage, err)
				} else {
					// 标记摘要已更新
					m.summaryTrigger.MarkSummaryUpdated(generateSessionKey(userID, sessionID))
				}
			}
		}
	}

	return nil
}

// analyzeAndCreateUserMemory 分析用户消息并创建记忆
func (m *MemoryManager) analyzeAndCreateUserMemory(ctx context.Context, userID, content string, parts []schema.MessageInputPart) {
	userMemoryList, err := m.storage.GetUserMemories(ctx, userID, 0, m.config.Retrieval)
	if err != nil {
		// 记忆创建失败不应该阻断主流程，只记录日志
		slog.Errorf("创建用户记忆失败: %v\n", err)
		return
	}

	// 构建完整的消息内容用于保存
	var fullContent strings.Builder
	if content != "" {
		fullContent.WriteString(content)
	}
	// 简单记录多媒体内容的存在
	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeImageURL:
			fullContent.WriteString(fmt.Sprintf("[图片]"))
			if part.Image.URL != nil {
				fullContent.WriteString(fmt.Sprintf(",link:%s", *part.Image.URL))
			}
		case schema.ChatMessagePartTypeAudioURL:
			fullContent.WriteString("[音频]")
			if part.Audio.URL != nil {
				fullContent.WriteString(fmt.Sprintf(",link:%s", *part.Audio.URL))
			}
		case schema.ChatMessagePartTypeVideoURL:
			fullContent.WriteString("[视频]")
			if part.Video.URL != nil {
				fullContent.WriteString(fmt.Sprintf(",link:%s", *part.Video.URL))
			}
		case schema.ChatMessagePartTypeFileURL:
			fullContent.WriteString("[文件]")
			if part.File.URL != nil {
				fullContent.WriteString(fmt.Sprintf(",link:%s", *part.File.URL))
			}
		}
	}

	classifierMemoryList, err := m.userMemoryAnalyzer.ShouldUpdateMemoryWithParts(ctx, content, parts, userMemoryList)
	if err != nil {
		// 记忆创建失败不应该阻断主流程，只记录日志
		slog.Errorf("创建用户记忆失败: %v\n", err)
		return
	}

	delIds := make([]string, 0)
	for _, v := range classifierMemoryList {
		if v.Op == UserMemoryAnalyzerOpDelete {
			delIds = append(delIds, v.Id)
		} else if v.Op == UserMemoryAnalyzerOpCreate {
			memory := &UserMemory{
				UserID: userID,
				Memory: v.Memory,
				Input:  fullContent.String(),
			}
			err = m.storage.SaveUserMemory(ctx, memory)
			if err != nil {
				// 记忆创建失败不应该阻断主流程，只记录日志
				slog.Errorf("创建用户记忆失败: %v\n", err)
			}
		} else if v.Op == UserMemoryAnalyzerOpUpdate {
			// 从已获取的记忆列表中查找现有记忆以保留CreatedAt
			var existingMemory *UserMemory
			for _, mem := range userMemoryList {
				if mem.ID == v.Id {
					existingMemory = mem
					break
				}
			}

			if existingMemory != nil {
				// 找到现有记忆，更新它，保留CreatedAt
				existingMemory.Memory = v.Memory
				existingMemory.Input = fullContent.String()
				err = m.storage.UpdateUserMemory(ctx, existingMemory)
			} else {
				// 没有找到现有记忆，创建新记忆
				memory := &UserMemory{
					ID:     v.Id,
					UserID: userID,
					Memory: v.Memory,
					Input:  fullContent.String(),
				}
				err = m.storage.SaveUserMemory(ctx, memory)
			}
			if err != nil {
				// 记忆更新失败不应该阻断主流程，只记录日志
				slog.Errorf("更新用户记忆失败: %v\n", err)
			}
		}
	}

	if len(delIds) > 0 {
		err = m.storage.DeleteUserMemoriesByIds(ctx, userID, delIds)
		if err != nil {
			// 记忆创建失败不应该阻断主流程，只记录日志
			slog.Errorf("创建用户记忆失败: %v\n", err)
		}
	}

	return
}

// shouldTriggerSummaryUpdate 判断是否需要触发摘要更新
func (m *MemoryManager) shouldTriggerSummaryUpdate(ctx context.Context, userID, sessionID string) (bool, error) {
	// 获取当前会话的消息总数
	messages, err := m.storage.GetMessages(ctx, sessionID, userID, 0) // 获取所有消息
	if err != nil {
		return false, fmt.Errorf("获取消息总数失败: %w", err)
	}

	messageCount := len(messages)
	sessionKey := generateSessionKey(userID, sessionID)

	return m.summaryTrigger.ShouldTriggerSummary(sessionKey, messageCount), nil
}

// updateSessionSummary 更新会话摘要（使用AI生成）
func (m *MemoryManager) updateSessionSummary(ctx context.Context, userID, sessionID string) error {
	// 获取最近的消息用于增量更新
	recentMessages, err := m.storage.GetMessages(ctx, sessionID, userID, 10) // 最近10条消息
	if err != nil {
		return err
	}

	if len(recentMessages) == 0 {
		return nil
	}

	// 检查是否已存在摘要
	existingSummary, err := m.storage.GetSessionSummary(ctx, sessionID, userID)
	if err != nil {
		return err
	}

	var summaryContent string
	if existingSummary != nil {
		// 使用增量摘要生成（基于现有摘要和最新消息）
		summaryContent, err = m.sessionSummaryGenerator.GenerateIncrementalSummary(
			ctx, recentMessages, existingSummary.Summary)
		if err != nil {
			return fmt.Errorf("生成增量摘要失败: %w", err)
		}

		// 更新现有摘要
		existingSummary.Summary = summaryContent
		return m.storage.UpdateSessionSummary(ctx, existingSummary)
	} else {
		// 获取更多历史消息用于生成完整摘要
		allMessages, err := m.storage.GetMessages(ctx, sessionID, userID, 20) // 最近20条消息
		if err != nil {
			return err
		}

		// 生成新摘要
		summaryContent, err = m.sessionSummaryGenerator.GenerateSummary(ctx, allMessages, "")
		if err != nil {
			return fmt.Errorf("生成新摘要失败: %w", err)
		}

		// 创建新摘要
		summary := &SessionSummary{
			SessionID: sessionID,
			UserID:    userID,
			Summary:   summaryContent,
		}
		return m.storage.SaveSessionSummary(ctx, summary)
	}
}

// GetUserMemories 获取用户记忆
func (m *MemoryManager) GetUserMemories(ctx context.Context, userID string) ([]*UserMemory, error) {
	return m.storage.GetUserMemories(ctx, userID, m.config.MemoryLimit, m.config.Retrieval)
}

// AddUserMemory 手动添加用户记忆
func (m *MemoryManager) AddUserMemory(ctx context.Context, userID, memoryContent, input string) error {
	memory := &UserMemory{
		UserID: userID,
		Memory: memoryContent,
		Input:  input,
	}

	return m.storage.SaveUserMemory(ctx, memory)
}

// UpdateUserMemory 更新用户记忆
func (m *MemoryManager) UpdateUserMemory(ctx context.Context, memory *UserMemory) error {
	return m.storage.UpdateUserMemory(ctx, memory)
}

// DeleteUserMemory 删除用户记忆
func (m *MemoryManager) DeleteUserMemory(ctx context.Context, memoryID string) error {
	return m.storage.DeleteUserMemory(ctx, memoryID)
}

// ClearUserMemories 清空用户记忆
func (m *MemoryManager) ClearUserMemories(ctx context.Context, userID string) error {
	return m.storage.ClearUserMemories(ctx, userID)
}

// SearchUserMemories 搜索用户记忆
func (m *MemoryManager) SearchUserMemories(ctx context.Context, userID, query string, limit int) ([]*UserMemory, error) {
	return m.storage.SearchUserMemories(ctx, userID, query, limit)
}

// GetSessionSummary 获取会话摘要
func (m *MemoryManager) GetSessionSummary(ctx context.Context, sessionID, userID string) (*SessionSummary, error) {
	return m.storage.GetSessionSummary(ctx, sessionID, userID)
}

// SaveMessage 保存消息
func (m *MemoryManager) SaveMessage(ctx context.Context, message *ConversationMessage) error {
	return m.storage.SaveMessage(ctx, message)
}

// GetMessages 获取会话消息
func (m *MemoryManager) GetMessages(ctx context.Context, sessionID, userID string, limit int) ([]*schema.Message, error) {
	messages, err := m.storage.GetMessages(ctx, sessionID, userID, limit)
	if err != nil {
		return nil, err
	}

	list := make([]*schema.Message, len(messages))
	for i, v := range messages {
		schemaMsg := &schema.Message{
			Role: schema.RoleType(v.Role),
		}
		schemaMsg.Content = v.Content
		if len(v.Parts) > 0 {
			multiContent := make([]schema.MessageInputPart, 0, len(v.Parts))
			multiContent = append(multiContent, v.Parts...)
			schemaMsg.UserInputMultiContent = multiContent
		}
		list[i] = schemaMsg
	}
	return list, nil
}

// GetConfig 获取配置
func (m *MemoryManager) GetConfig() *MemoryConfig {
	return m.config
}

// UpdateConfig 更新配置
func (m *MemoryManager) UpdateConfig(config *MemoryConfig) {
	if config != nil {
		m.config = config
		// 如果配置更新，重新启动定期清理
		m.cleanupCancel()
		cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
		m.cleanupCtx = cleanupCtx
		m.cleanupCancel = cleanupCancel
		m.startPeriodicCleanup()
	}
}

// GetMemoryStats 获取内存管理器统计信息
func (m *MemoryManager) GetMemoryStats() map[string]interface{} {
	stats := map[string]interface{}{
		"config": m.config,
	}

	// 如果启用异步处理，添加队列统计
	if m.config.AsyncProcessing {
		stats["taskQueue"] = m.GetTaskQueueStats()
	}

	// 添加会话状态统计（通过summary trigger获取）
	sessionCount := len(m.summaryTrigger.sessionStates)
	stats["activeSessions"] = sessionCount

	return stats
}

// ForceCleanupNow 强制立即执行清理
func (m *MemoryManager) ForceCleanupNow(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// 执行定期清理
	m.performPeriodicCleanup()

	return nil
}

// Close 关闭管理器
func (m *MemoryManager) Close() error {
	// 关闭定期清理任务
	if m.cleanupCancel != nil {
		m.cleanupCancel()
		// 等待清理goroutine结束
		m.cleanupWg.Wait()
	}

	// 关闭异步处理
	if m.config.AsyncProcessing {
		// 发送取消信号
		m.cancel()
		// 关闭任务通道
		close(m.taskChannel)
		// 等待所有goroutine结束
		m.wg.Wait()
	}

	return m.storage.Close()
}
