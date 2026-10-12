package builtin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	builtinsearch "github.com/CoolBanHub/aggo/memory/builtin/search"
	"github.com/CoolBanHub/aggo/utils"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/gookit/slog"
)

// ErrMemoryClosed 在管理器关闭后提交任务时返回。
var ErrMemoryClosed = errors.New("memory: manager is closed")

// managerState 描述管理器生命周期状态。
type managerState uint32

const (
	// managerRunning 正常接收并处理任务
	managerRunning managerState = iota
	// managerStopping 不再接收新任务，正在排空已接受的任务
	managerStopping
	// managerClosed 排空完成，底层存储已关闭
	managerClosed
)

// MemoryManager 记忆管理器
// 负责管理用户记忆、会话摘要和对话历史
type MemoryManager struct {
	// 存储接口
	storage MemoryStorage
	// 记忆配置
	config   *MemoryConfig
	configMu sync.RWMutex

	userMemoryAnalyzer      *UserMemoryAnalyzer
	sessionSummaryGenerator *SessionSummaryGenerator
	searcher                builtinsearch.Searcher

	// 摘要触发管理
	summaryTrigger *SummaryTriggerManager
	summaryCache   *sessionSummaryCache
	cacheMu        sync.RWMutex

	// 异步处理相关
	taskChannel chan asyncTask
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc

	// 生命周期状态。state 与 submitMu 配合：
	// 提交任务时在 submitMu 内检查 state，Shutdown 切换状态时也持有 submitMu，
	// 保证“通过检查的任务必然已入队”，不会出现提交后无人处理。
	state    atomic.Uint32
	submitMu sync.Mutex
	// inFlight 统计正在执行（已出队）的任务，Flush 依据其归零判断队列静止
	inFlight atomic.Int64

	// shutdownDone 在 doShutdown 完成后关闭；shutdownErr 保存其结果
	shutdownDone chan struct{}
	shutdownErr  error

	// 定期清理相关
	cleanupTicker *time.Ticker
	cleanupWg     sync.WaitGroup
	cleanupCtx    context.Context
	cleanupCancel context.CancelFunc
	cleanupMu     sync.Mutex

	// 异步任务队列统计
	taskQueueStats TaskQueueStats

	// 异步任务处理去重标记，防止同一(任务类型,用户,会话)多次排队
	pendingTasks sync.Map

	// 记忆任务聚合（debounce）相关
	memoryTimers   sync.Map      // key: memoryTimerKey, value: *time.Timer
	debounceWindow time.Duration // 聚合窗口时长，0 表示不做聚合

	// 外部注入的清理函数
	CleanupOldMessagesFunc     func(ctx context.Context) error // 按时间清理旧消息
	CleanupMessagesByLimitFunc func(ctx context.Context) error // 按数量限制清理消息

	asyncTaskContextBuilder AsyncTaskContextBuilder
}

// asyncTask 异步任务结构
type asyncTask struct {
	taskType  string // "memory" / "summary" / "index" / 自定义
	userID    string
	sessionID string
	message   *ConversationMessage
	// run 非空时执行该函数（EnqueueTask 提交的自定义任务），
	// 不参与 (taskType,userID,sessionID) 去重。
	run func(ctx context.Context)
}

type asyncTaskKey struct {
	taskType  string
	userID    string
	sessionID string
}

type memoryTimerKey struct {
	userID    string
	sessionID string
}

// NewMemoryManager 创建新的记忆管理器
func NewMemoryManager(cm model.AgenticModel, memoryStorage MemoryStorage, config *MemoryConfig) (*MemoryManager, error) {
	config = normalizeMemoryConfig(config)

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
		summaryCache: newSessionSummaryCache(
			time.Duration(config.SummaryCache.TTLSeconds)*time.Second,
			config.SummaryCache.MaxEntries,
		),
		ctx:            ctx,
		cancel:         cancel,
		cleanupCtx:     cleanupCtx,
		cleanupCancel:  cleanupCancel,
		debounceWindow: debounceWindowFromConfig(config),
		shutdownDone:   make(chan struct{}),
	}

	manager.searcher, err = newSearcher(memoryStorage, config.Search)
	if err != nil {
		cancel()
		cleanupCancel()
		return nil, err
	}

	// 事件检索模式需要底层存储实现 UserMemoryEventStorage；否则降级提示并关闭该模式。
	if config.EnableEventSearch {
		if _, ok := memoryStorage.(UserMemoryEventStorage); !ok {
			slog.Warnf("memory: EnableEventSearch=true 但当前 storage 未实现 UserMemoryEventStorage，将自动退化为兼容模式")
			config.EnableEventSearch = false
		} else {
			manager.userMemoryAnalyzer.SetUseEventSearchPrompt(true)
		}
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

func (m *MemoryManager) SetAsyncTaskContextBuilder(builder AsyncTaskContextBuilder) {
	m.asyncTaskContextBuilder = builder
}

func (m *MemoryManager) newAsyncTaskContext(task asyncTask) context.Context {
	if m.asyncTaskContextBuilder != nil {
		if ctx := m.asyncTaskContextBuilder(task.taskType, task.userID, task.sessionID); ctx != nil {
			return ctx
		}
	}
	return context.Background()
}

// startAsyncWorkers 启动异步工作goroutine池
// worker 收到退出信号后会先排空队列中已接受的任务再返回，
// 保证 Close/Shutdown 返回时已接受的任务全部完成。
func (m *MemoryManager) startAsyncWorkers() {
	cfg := m.configSnapshot()
	for i := 0; i < cfg.AsyncWorkerPoolSize; i++ {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			for {
				select {
				case <-m.ctx.Done():
					// 排空剩余队列后退出
					for {
						select {
						case task, ok := <-m.taskChannel:
							if !ok {
								return
							}
							m.runQueuedTask(task)
						default:
							return
						}
					}
				case task, ok := <-m.taskChannel:
					if !ok {
						return // channel 已关闭
					}
					m.runQueuedTask(task)
				}
			}
		}()
	}
	m.taskQueueStats.ActiveWorkers = cfg.AsyncWorkerPoolSize
}

func (m *MemoryManager) runQueuedTask(task asyncTask) {
	// 任务已取出准备处理，从排队重标记中移除，允许同类新任务入队
	taskKey := asyncTaskKey{taskType: task.taskType, userID: task.userID, sessionID: task.sessionID}
	m.pendingTasks.Delete(taskKey)

	// inFlight 在提交时增加，这里在任务实际执行完毕后减少
	defer m.inFlight.Add(-1)

	m.processAsyncTask(task)
	atomic.AddInt64(&m.taskQueueStats.ProcessedTasks, 1)
}

// submitAsyncTask 提交异步任务，带队列重防抖功能
func (m *MemoryManager) submitAsyncTask(task asyncTask) bool {
	taskKey := asyncTaskKey{taskType: task.taskType, userID: task.userID, sessionID: task.sessionID}
	// 如果相同签名（任务类型+用户+会话）的任务已在队列中，则丢弃当前重复提交，节省开销
	if _, loaded := m.pendingTasks.LoadOrStore(taskKey, struct{}{}); loaded {
		//slog.Debugf("异步任务去重: 已存在相同的待处理任务, 类型: %s, 用户: %s", task.taskType, task.userID)
		return true // 返回 true 表示"已接收处理"（虽然是去重扔掉的），不视为"队列满丢弃"
	}

	if !m.submitAsyncTaskRaw(task) {
		// 入队失败，清除去重标记以免卡死后续重试
		m.pendingTasks.Delete(taskKey)
		return false
	}
	return true
}

// submitAsyncTaskRaw 提交异步任务，不做去重。关闭后拒绝提交。
func (m *MemoryManager) submitAsyncTaskRaw(task asyncTask) bool {
	// 与 Shutdown 的状态切换互斥：通过检查的任务必然完成入队，
	// 不会出现“已排空退出后仍有任务滞留队列”的窗口。
	m.submitMu.Lock()
	defer m.submitMu.Unlock()

	if managerState(m.state.Load()) != managerRunning {
		atomic.AddInt64(&m.taskQueueStats.RejectedTasks, 1)
		return false
	}

	// Count before publishing to the buffered channel. A worker can receive a
	// task immediately, so counting afterwards allows Flush to observe a false
	// zero and return while the task is still running.
	m.inFlight.Add(1)
	select {
	case m.taskChannel <- task:
		return true
	default:
		m.inFlight.Add(-1)
		// 队列满，增加丢弃计数
		atomic.AddInt64(&m.taskQueueStats.DroppedTasks, 1)
		slog.Errorf("异步任务队列已满，丢弃任务. 队列: %d/%d, 总丢弃: %d, 任务类型: %s, 用户: %s",
			len(m.taskChannel), m.taskQueueStats.QueueCapacity,
			atomic.LoadInt64(&m.taskQueueStats.DroppedTasks),
			task.taskType, task.userID)
		return false
	}
}

// EnqueueTask 把 fn 提交到管理器的异步工作池执行。
// 任务与记忆分析任务共享生命周期：Flush/Shutdown 会等待其完成；
// 管理器关闭后返回 ErrMemoryClosed，队列满时返回错误。
// taskType 仅用于观测，不参与去重（每次调用都会执行）。
func (m *MemoryManager) EnqueueTask(taskType, userID, sessionID string, fn func(ctx context.Context)) error {
	if fn == nil {
		return errors.New("memory: task function is nil")
	}
	if !m.submitAsyncTaskRaw(asyncTask{
		taskType:  taskType,
		userID:    userID,
		sessionID: sessionID,
		run:       fn,
	}) {
		if managerState(m.state.Load()) != managerRunning {
			return ErrMemoryClosed
		}
		return fmt.Errorf("memory: async task queue is full (capacity %d)", m.taskQueueStats.QueueCapacity)
	}
	return nil
}

// Flush 等待已接受的任务全部完成：先触发未到期的 debounce 定时器，
// 再等待队列排空且没有正在执行的任务。ctx 取消时返回错误。
func (m *MemoryManager) Flush(ctx context.Context) error {
	if managerState(m.state.Load()) == managerClosed {
		return ErrMemoryClosed
	}

	// 触发未到期的聚合窗口，使 Flush 覆盖“已接受但还在 debounce 等待”的记忆请求
	m.firePendingDebounceTimers()

	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		// inFlight 在提交时增加、执行完毕后减少，
		// 为 0 表示已接受的任务（排队中或执行中）全部完成
		if m.inFlight.Load() == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("memory: flush interrupted: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// Shutdown 停止接收新任务，等待已接受的任务排空，最后关闭底层存储。
// 并发/重复调用安全：只有第一次调用执行关闭流程，其余调用等待其完成。
// ctx 取消时 Shutdown 返回错误，但关闭流程会在后台继续执行。
func (m *MemoryManager) Shutdown(ctx context.Context) error {
	m.submitMu.Lock()
	first := m.state.CompareAndSwap(uint32(managerRunning), uint32(managerStopping))
	m.submitMu.Unlock()

	if first {
		go m.doShutdown()
	}

	select {
	case <-m.shutdownDone:
		return m.shutdownErr
	case <-ctx.Done():
		return fmt.Errorf("memory: shutdown interrupted: %w", ctx.Err())
	}
}

// doShutdown 执行实际关闭流程，只应被调用一次。
// 关闭顺序：停止入口 → 停止 debounce 定时器 → 排空队列并等待 worker →
// 停止定期清理 → 关闭底层存储。
func (m *MemoryManager) doShutdown() {
	// Convert pending debounce windows into queue tasks before stopping workers.
	// These requests were already accepted by the middleware and must not be
	// lost merely because their debounce duration has not elapsed.
	m.flushPendingDebounceTimersForShutdown()
	m.stopMemoryTimers()

	// 通知 worker 退出；worker 会先排空队列再返回
	m.cancel()
	m.wg.Wait()

	// 停止定期清理任务
	m.cleanupMu.Lock()
	cleanupCancel := m.cleanupCancel
	m.cleanupMu.Unlock()
	cleanupCancel()
	m.cleanupWg.Wait()

	m.state.Store(uint32(managerClosed))
	m.shutdownErr = m.storage.Close()
	close(m.shutdownDone)
}

// Close 等待已接受任务完成后关闭管理器（Shutdown 的兼容包装，幂等）
func (m *MemoryManager) Close() error {
	return m.Shutdown(context.Background())
}

// stopMemoryTimers 停止所有聚合定时器
func (m *MemoryManager) stopMemoryTimers() {
	m.memoryTimers.Range(func(key, value any) bool {
		if timer, ok := value.(*time.Timer); ok {
			timer.Stop()
		}
		m.memoryTimers.Delete(key)
		return true
	})
}

// firePendingDebounceTimers 立即触发未到期的聚合定时器对应的任务。
// 使用带去重的 submitAsyncTask，与定时器回调本身的提交互斥，不会重复入队。
func (m *MemoryManager) firePendingDebounceTimers() {
	m.memoryTimers.Range(func(key, value any) bool {
		timerKey, ok := key.(memoryTimerKey)
		if !ok {
			return true
		}
		if timer, ok := value.(*time.Timer); ok {
			timer.Stop()
		}
		m.memoryTimers.Delete(timerKey)

		submitted := m.submitAsyncTask(asyncTask{
			taskType:  "memory",
			userID:    timerKey.userID,
			sessionID: timerKey.sessionID,
		})
		if !submitted {
			slog.Errorf("Flush 触发聚合任务失败: userID=%s sessionID=%s", timerKey.userID, timerKey.sessionID)
		}
		return true
	})
}

func (m *MemoryManager) flushPendingDebounceTimersForShutdown() {
	m.memoryTimers.Range(func(key, value any) bool {
		timerKey, ok := key.(memoryTimerKey)
		if !ok {
			return true
		}
		if timer, ok := value.(*time.Timer); ok {
			timer.Stop()
		}
		m.memoryTimers.Delete(key)
		// state 已单向进入 stopping（只有 doShutdown 会推进到 closed，
		// 且发生在本方法之后），无需再与提交门互斥。注意：不能在持有
		// submitMu 时阻塞发送——队列满且所有 worker 恰好都卡在需要
		// submitMu 的路径上时（如并发的 memorize 任务），会互相等待造成死锁。
		if managerState(m.state.Load()) != managerStopping {
			return true
		}
		// Workers remain alive until after this method returns, so blocking
		// here when the queue is full preserves the accepted task.
		m.inFlight.Add(1)
		m.taskChannel <- asyncTask{taskType: "memory", userID: timerKey.userID, sessionID: timerKey.sessionID}
		return true
	})
}

// scheduleMemoryTask 调度记忆分析任务，支持聚合窗口（debounce）
// 首次请求后启动 debounceWindow 定时器，期间的新请求不重置定时器，到期统一处理一次
func (m *MemoryManager) scheduleMemoryTask(userID, sessionID string) {
	m.configMu.RLock()
	debounceWindow := m.debounceWindow
	m.configMu.RUnlock()
	// debounceWindow 为 0 时，保持原有行为：立即提交
	if debounceWindow <= 0 {
		submitted := m.submitAsyncTask(asyncTask{
			taskType:  "memory",
			userID:    userID,
			sessionID: sessionID,
		})
		if !submitted {
			slog.Errorf("警告: 用户记忆分析队列已满，跳过处理: userID=%s\n", userID)
		}
		return
	}

	timerKey := memoryTimerKey{userID: userID, sessionID: sessionID}

	// Coordinate timer creation with Shutdown's state transition. Without this
	// gate a request can create a timer after shutdown has already flushed the
	// timer map, and its analysis would then be rejected and lost.
	m.submitMu.Lock()
	defer m.submitMu.Unlock()
	if managerState(m.state.Load()) != managerRunning {
		return
	}
	// 如果已有定时器，说明窗口内已有一次请求在等待，直接返回
	if _, loaded := m.memoryTimers.Load(timerKey); loaded {
		return
	}

	// 启动延迟定时器
	timer := time.AfterFunc(debounceWindow, func() {
		m.memoryTimers.Delete(timerKey)
		submitted := m.submitAsyncTask(asyncTask{
			taskType:  "memory",
			userID:    userID,
			sessionID: sessionID,
		})
		if !submitted {
			slog.Errorf("警告: 用户记忆分析队列已满，跳过处理: userID=%s\n", userID)
		}
	})
	m.memoryTimers.Store(timerKey, timer)
}

// GetTaskQueueStats 获取异步任务队列统计
func (m *MemoryManager) GetTaskQueueStats() TaskQueueStats {
	stats := TaskQueueStats{
		QueueCapacity:  m.taskQueueStats.QueueCapacity,
		ActiveWorkers:  m.taskQueueStats.ActiveWorkers,
		ProcessedTasks: atomic.LoadInt64(&m.taskQueueStats.ProcessedTasks),
		DroppedTasks:   atomic.LoadInt64(&m.taskQueueStats.DroppedTasks),
		RejectedTasks:  atomic.LoadInt64(&m.taskQueueStats.RejectedTasks),
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
	cfg := m.configSnapshot()
	ticker := time.NewTicker(time.Duration(cfg.Cleanup.CleanupInterval) * time.Hour)
	m.cleanupMu.Lock()
	m.cleanupTicker = ticker
	cleanupCtx := m.cleanupCtx
	m.cleanupMu.Unlock()
	m.cleanupWg.Add(1)
	go func() {
		defer m.cleanupWg.Done()
		for {
			select {
			case <-cleanupCtx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				m.performPeriodicCleanup(cleanupCtx)
			}
		}
	}()
}

// performPeriodicCleanup 执行定期清理
func (m *MemoryManager) performPeriodicCleanup(parentCtx context.Context) {
	cfg := m.configSnapshot()
	// 创建超时context，避免清理任务阻塞
	ctx, cancel := context.WithTimeout(parentCtx, 10*time.Minute)
	defer cancel()

	// 1. 清理旧的会话状态
	if cfg.Cleanup.SessionCleanupInterval > 0 {
		sessionRetention := time.Duration(cfg.Cleanup.SessionRetentionTime) * time.Hour
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
	// EnqueueTask 提交的自定义任务（如异步 Memorize）
	if task.run != nil {
		ctx, cancel := context.WithTimeout(m.newAsyncTaskContext(task), m.asyncTaskTimeout())
		defer cancel()
		task.run(ctx)
		return
	}

	switch task.taskType {
	case "memory":
		ctx, cancel := context.WithTimeout(m.newAsyncTaskContext(task), m.asyncTaskTimeout())
		defer cancel()
		m.analyzeAndCreateUserMemory(ctx, task.userID, task.sessionID)
	case "index":
		if task.message == nil {
			return
		}
		ctx, cancel := context.WithTimeout(m.newAsyncTaskContext(task), m.asyncTaskTimeout())
		defer cancel()
		if err := m.searcher.Index(ctx, toSearchMessage(task.message)); err != nil {
			slog.Errorf("异步建立搜索索引失败: sessionID=%s, userID=%s, err=%v\n", task.sessionID, task.userID, err)
		}
	case "summary":
		ctx, cancel := context.WithTimeout(m.newAsyncTaskContext(task), m.asyncTaskTimeout())
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
	cfg := m.configSnapshot()
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
	if cfg.Cleanup.MessageHistoryLimit > 0 {
		currentCount, err := m.storage.GetMessageCount(ctx, userID, sessionID)
		if err != nil {
			slog.Errorf("获取消息数量失败: %v", err)
		} else if currentCount >= cfg.Cleanup.MessageHistoryLimit {
			// 清理超出限制的消息，保留最新的N条
			err := m.storage.CleanupMessagesByLimit(ctx, userID, sessionID, cfg.Cleanup.MessageHistoryLimit-1)
			if err != nil {
				slog.Errorf("清理超限消息失败: %v", err)
			} else {
				slog.Infof("会话 %s 消息数量达到限制 %d，已清理旧消息", sessionID, cfg.Cleanup.MessageHistoryLimit)
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
	cfg := m.configSnapshot()
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
	if cfg.EnableSessionSummary {
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
	if cfg.EnableUserMemories {
		m.scheduleMemoryTask(userID, sessionID)
	}

	return nil
}

// analyzeAndCreateUserMemory 分析用户消息并更新记忆
func (m *MemoryManager) analyzeAndCreateUserMemory(ctx context.Context, userID, sessionID string) {
	cfg := m.configSnapshot()
	// 获取现有记忆
	existingMemory, err := m.storage.GetUserMemory(ctx, userID)
	if err != nil {
		slog.Errorf("获取用户记忆失败: %v\n", err)
		return
	}

	// 获取最近消息作为上下文
	historyMessages, err := m.storage.GetMessages(ctx, sessionID, userID, cfg.MemoryLimit/2)
	if err != nil {
		slog.Errorf("获取历史消息失败: %v\n", err)
		return
	}

	if len(historyMessages) == 0 {
		return
	}

	useEvent := cfg.EnableEventSearch
	req := AnalyzeRequest{
		ExistingMemory:  existingMemory,
		HistoryMessages: historyMessages,
		UseEventSearch:  &useEvent,
	}
	if useEvent {
		// 提供最近事件作为去重参考；条数比注入模型上下文时再多一点，给 LLM 足够上下文判断重复。
		recentLimit := cfg.RecentEventLimit * 2
		if recentLimit <= 0 {
			recentLimit = 20
		}
		if recent, err := m.ListRecentUserMemoryEvents(ctx, userID, recentLimit); err == nil {
			req.RecentEvents = recent
		} else {
			slog.Errorf("获取最近用户记忆事件失败: %v", err)
		}
	}

	result, err := m.userMemoryAnalyzer.AnalyzeOnce(ctx, req)
	if err != nil {
		slog.Errorf("分析用户记忆失败: %v\n", err)
		return
	}
	if result == nil || !result.NeedUpdate {
		return
	}

	// 短文档为空表示用户没有触发约定/基础信息更新，但事件可能仍然需要写入。
	if strings.TrimSpace(result.Memory) != "" {
		mem := &UserMemory{
			UserID: userID,
			Memory: result.Memory,
		}
		if existingMemory != nil {
			mem.CreatedAt = existingMemory.CreatedAt
		}
		if err := m.storage.UpsertUserMemory(ctx, mem); err != nil {
			slog.Errorf("保存用户记忆失败: %v\n", err)
		}
	}

	if useEvent && len(result.Events) > 0 {
		for _, evt := range result.Events {
			if evt == nil {
				continue
			}
			evt.UserID = userID
			if err := m.SaveUserMemoryEvent(ctx, evt); err != nil {
				slog.Errorf("保存用户记忆事件失败: %v\n", err)
			}
		}
	}
}

// userMemoryEventStorage 获取实现了事件存储接口的底层存储，未实现时返回 nil。
func (m *MemoryManager) userMemoryEventStorage() UserMemoryEventStorage {
	s, _ := m.storage.(UserMemoryEventStorage)
	return s
}

// SaveUserMemoryEvent 保存一条用户记忆事件。底层 storage 未实现事件接口时返回错误。
func (m *MemoryManager) SaveUserMemoryEvent(ctx context.Context, event *UserMemoryEvent) error {
	store := m.userMemoryEventStorage()
	if store == nil {
		return fmt.Errorf("当前 storage 未实现 UserMemoryEventStorage")
	}
	return store.SaveUserMemoryEvent(ctx, event)
}

// ListRecentUserMemoryEvents 返回用户最近的事件
func (m *MemoryManager) ListRecentUserMemoryEvents(ctx context.Context, userID string, limit int) ([]*UserMemoryEvent, error) {
	store := m.userMemoryEventStorage()
	if store == nil {
		return nil, nil
	}
	return store.ListRecentUserMemoryEvents(ctx, userID, limit)
}

// SearchUserMemoryEvents 按条件检索用户事件
func (m *MemoryManager) SearchUserMemoryEvents(ctx context.Context, query *UserMemoryEventQuery) ([]*UserMemoryEvent, error) {
	store := m.userMemoryEventStorage()
	if store == nil {
		return nil, fmt.Errorf("当前 storage 未实现 UserMemoryEventStorage")
	}
	return store.SearchUserMemoryEvents(ctx, query)
}

// shouldTriggerSummaryUpdate 判断是否需要触发摘要更新
func (m *MemoryManager) shouldTriggerSummaryUpdate(ctx context.Context, userID, sessionID string) (bool, error) {
	existingSummary, err := m.GetSessionSummary(ctx, sessionID, userID)
	if err != nil {
		return false, fmt.Errorf("获取会话摘要失败: %w", err)
	}

	if existingSummary == nil {
		totalMessageCount, err := m.storage.GetMessageCount(ctx, userID, sessionID)
		if err != nil {
			return false, fmt.Errorf("获取消息总数失败: %w", err)
		}
		if totalMessageCount == 0 {
			return false, nil
		}
		return m.shouldTriggerSummaryBySnapshot(time.Time{}, totalMessageCount, totalMessageCount, false), nil
	}

	unsummarizedCount, err := m.getUnsummarizedMessageCount(
		ctx,
		sessionID,
		userID,
		existingSummary.LastSummarizedMessageID,
		effectiveSummaryBoundary(existingSummary),
	)
	if err != nil {
		return false, fmt.Errorf("获取未摘要消息数量失败: %w", err)
	}

	totalMessageCount, err := m.storage.GetMessageCount(ctx, userID, sessionID)
	if err != nil {
		return false, fmt.Errorf("获取消息总数失败: %w", err)
	}

	return m.shouldTriggerSummaryBySnapshot(existingSummary.UpdatedAt, unsummarizedCount, totalMessageCount, true), nil
}

// updateSessionSummary 更新会话摘要（使用AI生成）
func (m *MemoryManager) updateSessionSummary(ctx context.Context, userID, sessionID string) error {
	// 检查是否已存在摘要
	existingSummary, err := m.GetSessionSummary(ctx, sessionID, userID)
	if err != nil {
		return err
	}

	var summaryContent string
	if existingSummary != nil {
		recentMessages, err := m.getMessagesAfterCursor(
			ctx,
			sessionID,
			userID,
			existingSummary.LastSummarizedMessageID,
			effectiveSummaryBoundary(existingSummary),
			0,
		)
		if err != nil {
			return fmt.Errorf("获取未摘要消息失败: %w", err)
		}
		if len(recentMessages) == 0 {
			return nil
		}

		// 使用增量摘要生成（基于现有摘要和游标后的最新消息）
		summaryContent, err = m.sessionSummaryGenerator.GenerateIncrementalSummary(
			ctx, recentMessages, existingSummary.Summary)
		if err != nil {
			return fmt.Errorf("生成增量摘要失败: %w", err)
		}

		// 更新现有摘要
		existingSummary.Summary = summaryContent
		lastMessage := recentMessages[len(recentMessages)-1]
		existingSummary.LastSummarizedMessageID = lastMessage.ID
		existingSummary.LastSummarizedMessageAt = lastMessage.CreatedAt
		if err := m.storage.UpdateSessionSummary(ctx, existingSummary); err != nil {
			return err
		}
		m.cacheSessionSummary(existingSummary)
		return nil
	} else {
		allMessages, err := m.storage.GetMessages(ctx, sessionID, userID, 0)
		if err != nil {
			return err
		}
		if len(allMessages) == 0 {
			return nil
		}

		// 生成新摘要
		summaryContent, err = m.sessionSummaryGenerator.GenerateSummary(ctx, allMessages, "")
		if err != nil {
			return fmt.Errorf("生成新摘要失败: %w", err)
		}

		lastMessage := allMessages[len(allMessages)-1]
		// 创建新摘要
		summary := &SessionSummary{
			SessionID:               sessionID,
			UserID:                  userID,
			Summary:                 summaryContent,
			LastSummarizedMessageID: lastMessage.ID,
			LastSummarizedMessageAt: lastMessage.CreatedAt,
		}
		if err := m.storage.SaveSessionSummary(ctx, summary); err != nil {
			return err
		}
		m.cacheSessionSummary(summary)
		return nil
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
	sessionKey := generateSessionKey(userID, sessionID)
	m.cacheMu.RLock()
	cache := m.summaryCache
	m.cacheMu.RUnlock()
	if cache != nil {
		if summary, ok := cache.Get(sessionKey); ok {
			return summary, nil
		}
	}

	summary, err := m.storage.GetSessionSummary(ctx, sessionID, userID)
	if err != nil || summary == nil {
		return summary, err
	}
	m.cacheSessionSummary(summary)
	return cloneSessionSummary(summary), nil
}

// SaveMessage 保存消息
func (m *MemoryManager) SaveMessage(ctx context.Context, message *ConversationMessage) error {
	if message == nil {
		return errors.New("消息对象不能为空")
	}
	// Allocate identity metadata before persistence and enqueue the exact
	// snapshot. Storage implementations are allowed to clone their input, so
	// indexing the caller's original object can otherwise see an empty ID/time.
	snapshot := *message
	if snapshot.ID == "" {
		snapshot.ID = utils.GetULID()
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now()
	}
	if message.Parts != nil {
		snapshot.Parts = append([]schema.MessageInputPart(nil), message.Parts...)
	}
	if err := m.storage.SaveMessage(ctx, &snapshot); err != nil {
		return err
	}
	enqueueIndexTask(m, &snapshot)
	return nil
}

// GetMessages 获取会话消息
func (m *MemoryManager) GetMessages(ctx context.Context, sessionID, userID string, limit int) ([]*schema.AgenticMessage, error) {
	messages, err := m.storage.GetMessages(ctx, sessionID, userID, limit)
	if err != nil {
		return nil, err
	}

	list := make([]*schema.AgenticMessage, len(messages))
	for i, v := range messages {
		list[i] = v.ToAgenticMessage()
	}
	return list, nil
}

// GetMessagesAfterSummary returns only the messages that have not yet been folded into the persisted session summary.
// For legacy summaries without a cursor, it falls back to messages created after the summary update time.
func (m *MemoryManager) GetMessagesAfterSummary(ctx context.Context, sessionID, userID string, limit int) ([]*schema.AgenticMessage, error) {
	summary, err := m.GetSessionSummary(ctx, sessionID, userID)
	if err != nil {
		return nil, err
	}

	var afterID string
	var afterTime time.Time
	if summary != nil {
		afterID = summary.LastSummarizedMessageID
		afterTime = effectiveSummaryBoundary(summary)
	}

	messages, err := m.getMessagesAfterCursor(ctx, sessionID, userID, afterID, afterTime, limit)
	if err != nil {
		return nil, err
	}

	list := make([]*schema.AgenticMessage, len(messages))
	for i, v := range messages {
		list[i] = v.ToAgenticMessage()
	}
	return list, nil
}

// GetConfig 获取配置
func (m *MemoryManager) GetConfig() *MemoryConfig {
	return cloneMemoryConfig(m.configSnapshot())
}

func (m *MemoryManager) configSnapshot() *MemoryConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	return m.config
}

// UpdateConfig 更新配置
func (m *MemoryManager) UpdateConfig(config *MemoryConfig) {
	if config == nil {
		return
	}
	// 关闭/关闭中不接受配置更新：会重启清理 goroutine 并写已关闭的存储
	if managerState(m.state.Load()) != managerRunning {
		return
	}

	config = cloneMemoryConfig(config)
	currentCacheTTLSeconds := 0
	m.configMu.RLock()
	current := m.config
	m.configMu.RUnlock()
	currentCacheMaxEntries := 0
	m.cacheMu.RLock()
	cache := m.summaryCache
	m.cacheMu.RUnlock()
	if cache != nil {
		currentCacheTTLSeconds = int(cache.ttl / time.Second)
		currentCacheMaxEntries = cache.maxEntries
	} else if current != nil {
		currentCacheTTLSeconds = current.SummaryCache.TTLSeconds
		currentCacheMaxEntries = current.SummaryCache.MaxEntries
	}

	config = normalizeMemoryConfig(config)
	m.configMu.Lock()
	m.config = config
	m.configMu.Unlock()
	m.summaryTrigger.UpdateConfig(config.SummaryTrigger)
	m.configMu.Lock()
	m.debounceWindow = debounceWindowFromConfig(config)
	m.configMu.Unlock()

	// Recreate the cache only when the currently applied cache parameters
	// differ from the requested ones. GetConfig returns an isolated snapshot.
	if currentCacheTTLSeconds != config.SummaryCache.TTLSeconds ||
		currentCacheMaxEntries != config.SummaryCache.MaxEntries {
		m.cacheMu.Lock()
		m.summaryCache = newSessionSummaryCache(
			time.Duration(config.SummaryCache.TTLSeconds)*time.Second,
			config.SummaryCache.MaxEntries,
		)
		m.cacheMu.Unlock()
	}

	// 停止旧的定期清理任务并等待退出
	m.cleanupMu.Lock()
	oldCleanupCancel := m.cleanupCancel
	m.cleanupMu.Unlock()
	oldCleanupCancel()
	m.cleanupWg.Wait()

	// 启动新的定期清理任务
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	m.cleanupMu.Lock()
	m.cleanupCtx = cleanupCtx
	m.cleanupCancel = cleanupCancel
	m.cleanupMu.Unlock()
	m.startPeriodicCleanup()
}

// GetMemoryStats 获取内存管理器统计信息
func (m *MemoryManager) GetMemoryStats() map[string]interface{} {
	stats := map[string]interface{}{
		"config": m.GetConfig(),
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

func (m *MemoryManager) cacheSessionSummary(summary *SessionSummary) {
	if summary == nil {
		return
	}
	m.cacheMu.RLock()
	cache := m.summaryCache
	m.cacheMu.RUnlock()
	if cache != nil {
		cache.Set(generateSessionKey(summary.UserID, summary.SessionID), summary)
	}
}

func (m *MemoryManager) shouldTriggerSummaryBySnapshot(lastSummaryTime time.Time, unsummarizedCount, totalMessageCount int, hasSummary bool) bool {
	if unsummarizedCount <= 0 && hasSummary {
		return false
	}

	cfg := m.summaryTrigger.configSnapshot()
	threshold := cfg.MessageThreshold
	if threshold <= 0 {
		threshold = DefaultMemoryConfig().SummaryTrigger.MessageThreshold
	}
	minInterval := cfg.MinInterval
	if minInterval <= 0 {
		minInterval = DefaultMemoryConfig().SummaryTrigger.MinInterval
	}

	switch cfg.Strategy {
	case TriggerAlways:
		if hasSummary {
			return unsummarizedCount > 0
		}
		return totalMessageCount >= threshold
	case TriggerByMessages:
		if hasSummary {
			return unsummarizedCount >= threshold
		}
		return totalMessageCount >= threshold
	case TriggerByTime:
		if !hasSummary {
			return totalMessageCount >= threshold
		}
		return unsummarizedCount > 0 && time.Since(lastSummaryTime).Seconds() >= float64(minInterval)
	case TriggerSmart:
		if !hasSummary {
			return totalMessageCount >= threshold
		}
		return m.summaryTrigger.shouldTriggerSmart(&SessionState{
			LastSummaryTime:          lastSummaryTime,
			MessagesSinceLastSummary: unsummarizedCount,
			TotalMessages:            totalMessageCount,
		})
	default:
		if hasSummary {
			return unsummarizedCount > 0
		}
		return totalMessageCount >= threshold
	}
}

func unsummarizedMessages(messages []*ConversationMessage, summary *SessionSummary) []*ConversationMessage {
	if len(messages) == 0 {
		return nil
	}
	if summary == nil {
		return messages
	}

	filtered := make([]*ConversationMessage, 0, len(messages))
	for _, msg := range messages {
		if messageAfterSummary(msg, summary) {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}

// messageAfterSummary reports whether message was created after the summary cursor.
//
// Cursor comparison uses two fields:
//   - LastSummarizedMessageAt (primary): timestamp of the last summarized message.
//   - LastSummarizedMessageID (tie-breaker): string comparison by ID for messages
//     sharing the same timestamp. This relies on IDs being lexicographically
//     monotonically increasing (e.g., ULID, UUIDv7). Random IDs would break this.
//
// For legacy summaries that predate cursor fields, falls back to UpdatedAt.
func messageAfterSummary(message *ConversationMessage, summary *SessionSummary) bool {
	if message == nil {
		return false
	}
	if summary == nil {
		return true
	}

	if summary.LastSummarizedMessageID != "" || !summary.LastSummarizedMessageAt.IsZero() {
		if !summary.LastSummarizedMessageAt.IsZero() {
			if message.CreatedAt.After(summary.LastSummarizedMessageAt) {
				return true
			}
			if message.CreatedAt.Before(summary.LastSummarizedMessageAt) {
				return false
			}
		}
		if summary.LastSummarizedMessageID != "" {
			return message.ID > summary.LastSummarizedMessageID
		}
		return false
	}

	// Legacy summaries created before cursor fields existed fall back to the
	// summary update time as the best available boundary.
	return message.CreatedAt.After(summary.UpdatedAt)
}

func effectiveSummaryBoundary(summary *SessionSummary) time.Time {
	if summary == nil {
		return time.Time{}
	}
	if !summary.LastSummarizedMessageAt.IsZero() {
		return summary.LastSummarizedMessageAt
	}
	return summary.UpdatedAt
}

// debounceWindowFromConfig extracts the debounce window duration from config.
func debounceWindowFromConfig(config *MemoryConfig) time.Duration {
	if config.DebounceWindowSeconds != nil {
		return time.Duration(*config.DebounceWindowSeconds) * time.Second
	}
	return 30 * time.Second
}

func (m *MemoryManager) asyncTaskTimeout() time.Duration {
	return time.Duration(m.configSnapshot().AsyncTaskTimeoutSeconds) * time.Second
}

func normalizeMemoryConfig(config *MemoryConfig) *MemoryConfig {
	if config == nil {
		config = DefaultMemoryConfig()
	}

	defaults := DefaultMemoryConfig()
	if config.MemoryLimit <= 0 {
		config.MemoryLimit = defaults.MemoryLimit
	}
	if config.SummaryRecentMessageLimit < 0 {
		config.SummaryRecentMessageLimit = 0
	}
	if config.RecentEventLimit < 0 {
		config.RecentEventLimit = 0
	} else if config.RecentEventLimit == 0 && config.EnableEventSearch {
		config.RecentEventLimit = defaults.RecentEventLimit
	}
	if config.AsyncWorkerPoolSize <= 0 {
		config.AsyncWorkerPoolSize = defaults.AsyncWorkerPoolSize
	}
	if config.DebounceWindowSeconds == nil {
		config.DebounceWindowSeconds = defaults.DebounceWindowSeconds
	}
	if config.AsyncTaskTimeoutSeconds <= 0 {
		config.AsyncTaskTimeoutSeconds = defaults.AsyncTaskTimeoutSeconds
	}
	if config.SummaryTrigger.MessageThreshold <= 0 {
		config.SummaryTrigger.MessageThreshold = defaults.SummaryTrigger.MessageThreshold
	}
	if config.SummaryTrigger.MinInterval <= 0 {
		config.SummaryTrigger.MinInterval = defaults.SummaryTrigger.MinInterval
	}
	if config.SummaryCache.TTLSeconds <= 0 {
		config.SummaryCache.TTLSeconds = defaults.SummaryCache.TTLSeconds
	}
	if config.SummaryCache.MaxEntries <= 0 {
		config.SummaryCache.MaxEntries = defaults.SummaryCache.MaxEntries
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
	config.Search = normalizeSearchConfig(config.Search)
	return config
}

func cloneMemoryConfig(src *MemoryConfig) *MemoryConfig {
	if src == nil {
		return nil
	}
	cloned := *src
	if src.DebounceWindowSeconds != nil {
		v := *src.DebounceWindowSeconds
		cloned.DebounceWindowSeconds = &v
	}
	if src.Search != nil {
		search := *src.Search
		cloned.Search = &search
	}
	return &cloned
}

// getMessagesAfterCursor returns messages after the summary cursor.
// It prefers the efficient CursorMessageStorage path when available.
// For stores that don't implement CursorMessageStorage (e.g., in-memory),
// it loads all messages and filters in-process — acceptable because the
// in-memory store is only used for testing/development.
func (m *MemoryManager) getMessagesAfterCursor(ctx context.Context, sessionID, userID, afterMessageID string, afterTime time.Time, limit int) ([]*ConversationMessage, error) {
	if cursorStore, ok := m.storage.(CursorMessageStorage); ok {
		return cursorStore.GetMessagesAfter(ctx, sessionID, userID, afterMessageID, afterTime, limit)
	}

	// Fallback: load all messages and filter in-process.
	messages, err := m.storage.GetMessages(ctx, sessionID, userID, 0)
	if err != nil {
		return nil, err
	}

	filtered := unsummarizedMessages(messages, &SessionSummary{
		LastSummarizedMessageID: afterMessageID,
		LastSummarizedMessageAt: afterTime,
		UpdatedAt:               afterTime,
	})
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered, nil
}

// getUnsummarizedMessageCount returns the count of messages after the summary cursor
// without loading the full message list. Falls back to loading all messages when
// the storage doesn't support efficient cursor counting.
func (m *MemoryManager) getUnsummarizedMessageCount(ctx context.Context, sessionID, userID, afterMessageID string, afterTime time.Time) (int, error) {
	if cursorStore, ok := m.storage.(CursorMessageStorage); ok {
		return cursorStore.GetMessageCountAfter(ctx, sessionID, userID, afterMessageID, afterTime)
	}

	// Fallback: load all messages and count in-process.
	messages, err := m.storage.GetMessages(ctx, sessionID, userID, 0)
	if err != nil {
		return 0, err
	}
	filtered := unsummarizedMessages(messages, &SessionSummary{
		LastSummarizedMessageID: afterMessageID,
		LastSummarizedMessageAt: afterTime,
		UpdatedAt:               afterTime,
	})
	return len(filtered), nil
}
