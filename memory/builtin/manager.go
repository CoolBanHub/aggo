package builtin

import (
	"context"
	"errors"
	"fmt"
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

	// 异步任务处理去重标记，防止同一(任务类型,用户,会话)多次排队
	pendingTasks sync.Map

	// 外部注入的清理函数
	CleanupOldMessagesFunc     func(ctx context.Context) error // 按时间清理旧消息
	CleanupMessagesByLimitFunc func(ctx context.Context) error // 按数量限制清理消息
}

// asyncTask 异步任务结构
type asyncTask struct {
	taskType  string // "memory" 或 "summary"
	userID    string
	sessionID string
}

// NewMemoryManager 创建新的记忆管理器
func NewMemoryManager(cm model.ToolCallingChatModel, memoryStorage MemoryStorage, config *MemoryConfig) (*MemoryManager, error) {
	if config == nil {
		config = DefaultMemoryConfig()
	}

	// 填充零值字段的默认值
	defaults := DefaultMemoryConfig()
	if config.MemoryLimit <= 0 {
		config.MemoryLimit = defaults.MemoryLimit
	}
	if config.AsyncWorkerPoolSize <= 0 {
		config.AsyncWorkerPoolSize = defaults.AsyncWorkerPoolSize
	}
	if config.SummaryTrigger.MessageThreshold <= 0 {
		config.SummaryTrigger.MessageThreshold = defaults.SummaryTrigger.MessageThreshold
	}
	if config.Cleanup.CleanupInterval <= 0 {
		config.Cleanup.CleanupInterval = defaults.Cleanup.CleanupInterval
	}
	if config.Cleanup.SessionCleanupInterval <= 0 {
		config.Cleanup.SessionCleanupInterval = defaults.Cleanup.SessionCleanupInterval
	}
	if config.Cleanup.SessionRetentionTime <= 0 {
		config.Cleanup.SessionRetentionTime = defaults.Cleanup.SessionRetentionTime
	}
	if config.Cleanup.MessageHistoryLimit <= 0 {
		config.Cleanup.MessageHistoryLimit = defaults.Cleanup.MessageHistoryLimit
	}

	// 设置表前缀
	if config.TablePre != "" {
		memoryStorage.SetTablePrefix(config.TablePre)
	}

	err := memoryStorage.AutoMigrate()
	if err != nil {
		return nil, err
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

	// 初始化goroutine池
	queueCapacity := config.AsyncWorkerPoolSize * 10 // 缓冲区大小为工作池的10倍
	manager.taskChannel = make(chan asyncTask, queueCapacity)
	manager.taskQueueStats.QueueCapacity = queueCapacity
	manager.startAsyncWorkers()

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
				case task, ok := <-m.taskChannel:
					if !ok {
						return // channel 已关闭
					}

					// 任务已取出准备处理，从排队重标记中移除，允许同类新任务入队
					taskKey := fmt.Sprintf("%s:%s:%s", task.taskType, task.userID, task.sessionID)
					m.pendingTasks.Delete(taskKey)

					m.processAsyncTask(task)
					atomic.AddInt64(&m.taskQueueStats.ProcessedTasks, 1)
				}
			}
		}()
	}
	m.taskQueueStats.ActiveWorkers = m.config.AsyncWorkerPoolSize
}

// submitAsyncTask 提交异步任务，带队列重防抖功能
func (m *MemoryManager) submitAsyncTask(task asyncTask) bool {
	taskKey := fmt.Sprintf("%s:%s:%s", task.taskType, task.userID, task.sessionID)
	// 如果相同签名（任务类型+用户+会话）的任务已在队列中，则丢弃当前重复提交，节省开销
	if _, loaded := m.pendingTasks.LoadOrStore(taskKey, struct{}{}); loaded {
		slog.Debugf("异步任务去重: 已存在相同的待处理任务, 类型: %s, 用户: %s", task.taskType, task.userID)
		return true // 返回 true 表示"已接收处理"（虽然是去重扔掉的），不视为"队列满丢弃"
	}

	select {
	case m.taskChannel <- task:
		return true
	default:
		// 队列满，入队失败，需清除去重标记以免卡死后续重试
		m.pendingTasks.Delete(taskKey)

		// 队列满，增加丢弃计数
		atomic.AddInt64(&m.taskQueueStats.DroppedTasks, 1)
		slog.Errorf("异步任务队列已满，丢弃任务. 队列: %d/%d, 总丢弃: %d, 任务类型: %s, 用户: %s",
			len(m.taskChannel), m.taskQueueStats.QueueCapacity,
			atomic.LoadInt64(&m.taskQueueStats.DroppedTasks),
			task.taskType, task.userID)
		return false
	}
}

// GetTaskQueueStats 获取异步任务队列统计
func (m *MemoryManager) GetTaskQueueStats() TaskQueueStats {
	stats := TaskQueueStats{
		QueueCapacity:  m.taskQueueStats.QueueCapacity,
		ActiveWorkers:  m.taskQueueStats.ActiveWorkers,
		ProcessedTasks: atomic.LoadInt64(&m.taskQueueStats.ProcessedTasks),
		DroppedTasks:   atomic.LoadInt64(&m.taskQueueStats.DroppedTasks),
	}
	if m.taskChannel != nil {
		stats.QueueSize = len(m.taskChannel)
		if stats.QueueCapacity > 0 {
			stats.QueueUtilization = float64(stats.QueueSize) / float64(stats.QueueCapacity)
		}
	}
	return stats
}

// startPeriodicCleanup 启动定期清理任务
func (m *MemoryManager) startPeriodicCleanup() {
	m.cleanupTicker = time.NewTicker(time.Duration(m.config.Cleanup.CleanupInterval) * time.Hour)
	m.cleanupWg.Add(1)
	go func() {
		defer m.cleanupWg.Done()
		for {
			select {
			case <-m.cleanupCtx.Done():
				m.cleanupTicker.Stop()
				return
			case <-m.cleanupTicker.C:
				m.performPeriodicCleanup(m.cleanupCtx)
			}
		}
	}()
}

// performPeriodicCleanup 执行定期清理
func (m *MemoryManager) performPeriodicCleanup(parentCtx context.Context) {
	// 创建超时context，避免清理任务阻塞
	ctx, cancel := context.WithTimeout(parentCtx, 10*time.Minute)
	defer cancel()

	// 1. 清理旧的会话状态
	if m.config.Cleanup.SessionCleanupInterval > 0 {
		sessionRetention := time.Duration(m.config.Cleanup.SessionRetentionTime) * time.Hour
		m.summaryTrigger.CleanupOldSessions(sessionRetention)
	}

	// 2. 清理旧的消息历史（按时间）- 调用外部注入的函数
	if m.CleanupOldMessagesFunc != nil {
		if err := m.CleanupOldMessagesFunc(ctx); err != nil {
			slog.Errorf("清理旧消息失败: %v", err)
		}
	}

	// 3. 按数量限制清理消息 - 调用外部注入的函数
	if m.CleanupMessagesByLimitFunc != nil {
		if err := m.CleanupMessagesByLimitFunc(ctx); err != nil {
			slog.Errorf("按数量清理消息失败: %v", err)
		}
	}
}

// processAsyncTask 处理异步任务
func (m *MemoryManager) processAsyncTask(task asyncTask) {
	switch task.taskType {
	case "memory":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		m.analyzeAndCreateUserMemory(ctx, task.userID, task.sessionID)
	case "summary":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
func (m *MemoryManager) ProcessUserMessage(ctx context.Context, userID, sessionID, content string, parts []schema.MessageInputPart) error {
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
	if m.config.Cleanup.MessageHistoryLimit > 0 {
		currentCount, err := m.storage.GetMessageCount(ctx, userID, sessionID)
		if err != nil {
			slog.Errorf("获取消息数量失败: %v", err)
		} else if currentCount >= m.config.Cleanup.MessageHistoryLimit {
			// 清理超出限制的消息，保留最新的N条
			err := m.storage.CleanupMessagesByLimit(ctx, userID, sessionID, m.config.Cleanup.MessageHistoryLimit-1)
			if err != nil {
				slog.Errorf("清理超限消息失败: %v", err)
			} else {
				slog.Infof("会话 %s 消息数量达到限制 %d，已清理旧消息", sessionID, m.config.Cleanup.MessageHistoryLimit)
			}
		}
	}

	// 保存用户消息到对话历史
	err := m.SaveMessage(ctx, &ConversationMessage{
		SessionID: sessionID,
		UserID:    userID,
		Role:      "user",
		Content:   content,
		Parts:     parts,
	})
	if err != nil {
		return fmt.Errorf("保存用户消息失败: %v", err)
	}

	return nil
}

// ProcessAssistantMessage 处理助手回复消息
func (m *MemoryManager) ProcessAssistantMessage(ctx context.Context, userID, sessionID, assistantMessage string) error {
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
			submitted := m.submitAsyncTask(asyncTask{
				taskType:  "summary",
				userID:    userID,
				sessionID: sessionID,
			})
			if !submitted {
				slog.Errorf("警告: 会话摘要更新队列已满，跳过处理: sessionID=%s, userID=%s\n", sessionID, userID)
			}
		}
	}

	// 如果启用了用户记忆，分析消息并创建记忆（在AI回复后触发）
	if m.config.EnableUserMemories {
		submitted := m.submitAsyncTask(asyncTask{
			taskType:  "memory",
			userID:    userID,
			sessionID: sessionID,
		})
		if !submitted {
			slog.Errorf("警告: 用户记忆分析队列已满，跳过处理: userID=%s\n", userID)
		}
	}

	return nil
}

// analyzeAndCreateUserMemory 分析用户消息并更新记忆
func (m *MemoryManager) analyzeAndCreateUserMemory(ctx context.Context, userID, sessionID string) {
	// 获取现有记忆
	existingMemory, err := m.storage.GetUserMemory(ctx, userID)
	if err != nil {
		slog.Errorf("获取用户记忆失败: %v\n", err)
		return
	}

	// 获取最近消息作为上下文
	historyMessages, err := m.storage.GetMessages(ctx, sessionID, userID, m.config.MemoryLimit/2)
	if err != nil {
		slog.Errorf("获取历史消息失败: %v\n", err)
		return
	}

	if len(historyMessages) == 0 {
		return
	}

	// 分析对话并生成更新后的记忆
	needUpdate, newMemoryContent, err := m.userMemoryAnalyzer.ShouldUpdateMemory(
		ctx,
		existingMemory,
		historyMessages,
	)
	if err != nil {
		slog.Errorf("分析用户记忆失败: %v\n", err)
		return
	}

	// 如果不需要更新，直接返回
	if !needUpdate {
		return
	}

	// 保存更新后的记忆
	mem := &UserMemory{
		UserID: userID,
		Memory: newMemoryContent,
	}

	// 如果有现有记忆，保留创建时间
	if existingMemory != nil {
		mem.CreatedAt = existingMemory.CreatedAt
	}

	err = m.storage.UpsertUserMemory(ctx, mem)
	if err != nil {
		slog.Errorf("保存用户记忆失败: %v\n", err)
	}
}

// shouldTriggerSummaryUpdate 判断是否需要触发摘要更新
func (m *MemoryManager) shouldTriggerSummaryUpdate(ctx context.Context, userID, sessionID string) (bool, error) {
	// 直接获取消息数量，避免加载全量消息到内存
	messageCount, err := m.storage.GetMessageCount(ctx, userID, sessionID)
	if err != nil {
		return false, fmt.Errorf("获取消息总数失败: %w", err)
	}

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

// GetUserMemory 获取用户记忆
func (m *MemoryManager) GetUserMemory(ctx context.Context, userID string) (*UserMemory, error) {
	return m.storage.GetUserMemory(ctx, userID)
}

// UpsertUserMemory 创建或更新用户记忆
func (m *MemoryManager) UpsertUserMemory(ctx context.Context, memory *UserMemory) error {
	return m.storage.UpsertUserMemory(ctx, memory)
}

// ClearUserMemory 清空用户记忆
func (m *MemoryManager) ClearUserMemory(ctx context.Context, userID string) error {
	return m.storage.ClearUserMemory(ctx, userID)
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
		list[i] = v.ToSchemaMessage()
	}
	return list, nil
}

// GetConfig 获取配置
func (m *MemoryManager) GetConfig() *MemoryConfig {
	return m.config
}

// UpdateConfig 更新配置
func (m *MemoryManager) UpdateConfig(config *MemoryConfig) {
	if config == nil {
		return
	}

	m.config = config

	// 停止旧的定期清理任务并等待退出
	m.cleanupCancel()
	m.cleanupWg.Wait()

	// 启动新的定期清理任务
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	m.cleanupCtx = cleanupCtx
	m.cleanupCancel = cleanupCancel
	m.startPeriodicCleanup()
}

// GetMemoryStats 获取内存管理器统计信息
func (m *MemoryManager) GetMemoryStats() map[string]interface{} {
	stats := map[string]interface{}{
		"config": m.config,
	}

	// 添加队列统计
	stats["taskQueue"] = m.GetTaskQueueStats()

	// 添加会话状态统计（通过并发安全的方法获取）
	stats["activeSessions"] = m.summaryTrigger.GetSessionCount()

	return stats
}

// ForceCleanupNow 强制立即执行清理
func (m *MemoryManager) ForceCleanupNow(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// 使用传入的 ctx 执行清理
	m.performPeriodicCleanup(ctx)

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

	// 通知所有 worker 退出，等待退出后再关闭 channel
	m.cancel()
	m.wg.Wait()
	close(m.taskChannel)

	return m.storage.Close()
}
