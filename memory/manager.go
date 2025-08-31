package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
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
}

// asyncTask 异步任务结构
type asyncTask struct {
	taskType  string // "memory" 或 "summary"
	userID    string
	sessionID string
	message   string
}

// NewMemoryManager 创建新的记忆管理器
func NewMemoryManager(cm model.ToolCallingChatModel, storage MemoryStorage, config *MemoryConfig) *MemoryManager {
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
		}
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

	manager := &MemoryManager{
		storage:                 storage,
		config:                  config,
		userMemoryAnalyzer:      NewUserMemoryAnalyzer(cm),
		sessionSummaryGenerator: NewSessionSummaryGenerator(cm),
		summaryTrigger:          NewSummaryTriggerManager(config.SummaryTrigger),
		ctx:                     ctx,
		cancel:                  cancel,
	}

	// 如果启用异步处理，初始化goroutine池
	if config.AsyncProcessing {
		manager.taskChannel = make(chan asyncTask, config.AsyncWorkerPoolSize*2) // 缓冲区大小为工作池的2倍
		manager.startAsyncWorkers()
	}

	return manager
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
					m.processAsyncTask(task)
				}
			}
		}()
	}
}

// processAsyncTask 处理异步任务
func (m *MemoryManager) processAsyncTask(task asyncTask) {
	switch task.taskType {
	case "memory":
		// 创建新的context用于异步操作，避免超时
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // 30秒超时
		defer cancel()
		m.analyzeAndCreateUserMemory(ctx, task.userID, task.message)
	case "summary":
		// 创建新的context用于异步操作
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second) // 30秒超时
		defer cancel()
		err := m.updateSessionSummary(ctx, task.userID, task.sessionID)
		if err != nil {
			fmt.Printf("异步更新会话摘要失败: sessionID=%s, userID=%s, err=%v\n", task.sessionID, task.userID, err)
		} else {
			// 标记摘要已更新
			m.summaryTrigger.MarkSummaryUpdated(generateSessionKey(task.userID, task.sessionID))
		}
	}
}

// ProcessUserMessage 处理用户消息
// 根据配置决定是否创建用户记忆、更新会话摘要等
func (m *MemoryManager) ProcessUserMessage(ctx context.Context, userID, sessionID, userMessage string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if sessionID == "" {
		return errors.New("会话ID不能为空")
	}
	if userMessage == "" {
		return errors.New("用户消息不能为空")
	}

	// 保存用户消息到对话历史
	err := m.SaveMessage(ctx, &ConversationMessage{
		SessionID: sessionID,
		UserID:    userID,
		Role:      "user",
		Content:   userMessage,
	})
	if err != nil {
		return fmt.Errorf("保存用户消息失败: %v", err)
	}

	// 如果启用了用户记忆，分析消息并创建记忆
	if m.config.EnableUserMemories {
		if m.config.AsyncProcessing {
			// 异步处理
			select {
			case m.taskChannel <- asyncTask{
				taskType: "memory",
				userID:   userID,
				message:  userMessage,
			}:
				// 任务已提交到队列
			default:
				// 队列已满，记录日志但不阻塞
				fmt.Printf("警告: 用户记忆分析队列已满，跳过处理: userID=%s\n", userID)
			}
		} else {
			// 同步处理
			m.analyzeAndCreateUserMemory(ctx, userID, userMessage)
		}
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
		Role:      "assistant",
		Content:   assistantMessage,
	})
	if err != nil {
		return fmt.Errorf("保存助手消息失败: %v", err)
	}

	// 如果启用了会话摘要，检查是否需要更新摘要
	if m.config.EnableSessionSummary {
		shouldTrigger, err := m.shouldTriggerSummaryUpdate(ctx, userID, sessionID)
		if err != nil {
			fmt.Printf("检查摘要触发条件失败: %v\n", err)
		} else if shouldTrigger {
			if m.config.AsyncProcessing {
				// 异步处理
				select {
				case m.taskChannel <- asyncTask{
					taskType:  "summary",
					userID:    userID,
					sessionID: sessionID,
				}:
					// 任务已提交到队列
				default:
					// 队列已满，记录日志但不阻塞
					fmt.Printf("警告: 会话摘要更新队列已满，跳过处理: sessionID=%s, userID=%s\n", sessionID, userID)
				}
			} else {
				// 同步处理
				err = m.updateSessionSummary(ctx, userID, sessionID)
				if err != nil {
					fmt.Printf("更新会话摘要失败: msg:%s,err:%v\n", assistantMessage, err)
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
// 这是一个简化的实现，实际项目中可能需要使用AI模型来判断
func (m *MemoryManager) analyzeAndCreateUserMemory(ctx context.Context, userID, message string) {
	// 简单的规则判断是否需要创建记忆
	userMemoryList, err := m.storage.GetUserMemories(ctx, userID, 0, m.config.Retrieval)
	if err != nil {
		// 记忆创建失败不应该阻断主流程，只记录日志
		fmt.Printf("创建用户记忆失败: %v\n", err)
		return
	}

	classifierMemoryList, err := m.userMemoryAnalyzer.ShouldUpdateMemory(ctx, message, userMemoryList)
	if err != nil {
		// 记忆创建失败不应该阻断主流程，只记录日志
		fmt.Printf("创建用户记忆失败: %v\n", err)
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
				Input:  message,
			}
			err = m.storage.SaveUserMemory(ctx, memory)
			if err != nil {
				// 记忆创建失败不应该阻断主流程，只记录日志
				fmt.Printf("创建用户记忆失败: %v\n", err)
			}
		} else if v.Op == UserMemoryAnalyzerOpUpdate {
			err = m.storage.UpdateUserMemory(ctx, &UserMemory{
				ID:     v.Id,
				UserID: userID,
				Memory: v.Memory,
			})
			if err != nil {
				// 记忆创建失败不应该阻断主流程，只记录日志
				fmt.Printf("创建用户记忆失败: %v\n", err)
			}
		}
	}

	if len(delIds) > 0 {
		err = m.storage.DeleteUserMemoriesByIds(ctx, userID, delIds)
		if err != nil {
			// 记忆创建失败不应该阻断主流程，只记录日志
			fmt.Printf("创建用户记忆失败: %v\n", err)
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
		list[i] = &schema.Message{
			Role:    schema.RoleType(v.Role),
			Content: v.Content,
		}
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
	}
}

// Close 关闭管理器
func (m *MemoryManager) Close() error {
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
