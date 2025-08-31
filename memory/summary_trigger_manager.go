package memory

import (
	"sync"
	"time"
)

// SummaryTriggerManager 摘要触发管理器
type SummaryTriggerManager struct {
	config SummaryTriggerConfig

	// 记录每个会话的状态
	sessionStates map[string]*SessionState
	mutex         sync.RWMutex
}

// SessionState 会话状态
type SessionState struct {
	// 上次摘要更新时间
	LastSummaryTime time.Time
	// 上次摘要后新增的消息数量
	MessagesSinceLastSummary int
	// 会话的总消息数量
	TotalMessages int
}

// NewSummaryTriggerManager 创建新的摘要触发管理器
func NewSummaryTriggerManager(config SummaryTriggerConfig) *SummaryTriggerManager {
	// 设置默认值
	if config.MessageThreshold <= 0 {
		config.MessageThreshold = 10
	}
	if config.MinInterval <= 0 {
		config.MinInterval = 60
	}
	if config.Strategy == "" {
		config.Strategy = TriggerSmart
	}

	return &SummaryTriggerManager{
		config:        config,
		sessionStates: make(map[string]*SessionState),
	}
}

// ShouldTriggerSummary 判断是否应该触发摘要更新
func (stm *SummaryTriggerManager) ShouldTriggerSummary(sessionKey string, currentMessageCount int) bool {
	stm.mutex.Lock()
	defer stm.mutex.Unlock()

	// 获取或创建会话状态
	state, exists := stm.sessionStates[sessionKey]
	if !exists {
		state = &SessionState{
			LastSummaryTime:          time.Now(),
			MessagesSinceLastSummary: 0,
			TotalMessages:            currentMessageCount,
		}
		stm.sessionStates[sessionKey] = state

		// 新会话，如果有足够的消息就触发
		return currentMessageCount >= stm.config.MessageThreshold
	}

	// 更新状态
	newMessages := currentMessageCount - state.TotalMessages
	if newMessages > 0 {
		state.MessagesSinceLastSummary += newMessages
		state.TotalMessages = currentMessageCount
	}

	// 根据策略判断是否触发
	switch stm.config.Strategy {
	case TriggerAlways:
		return true

	case TriggerByMessages:
		return state.MessagesSinceLastSummary >= stm.config.MessageThreshold

	case TriggerByTime:
		timeSinceLastSummary := time.Since(state.LastSummaryTime)
		return timeSinceLastSummary.Seconds() >= float64(stm.config.MinInterval)

	case TriggerSmart:
		return stm.shouldTriggerSmart(state)

	default:
		return true
	}
}

// shouldTriggerSmart 智能触发判断
func (stm *SummaryTriggerManager) shouldTriggerSmart(state *SessionState) bool {
	now := time.Now()
	timeSinceLastSummary := now.Sub(state.LastSummaryTime)

	// 条件1：消息数量达到阈值
	messageCondition := state.MessagesSinceLastSummary >= stm.config.MessageThreshold

	// 条件2：时间间隔足够长
	timeCondition := timeSinceLastSummary.Seconds() >= float64(stm.config.MinInterval)

	// 条件3：避免过于频繁的更新（至少30秒间隔）
	minTimeGap := timeSinceLastSummary.Seconds() >= 30

	// 智能策略：满足消息条件且时间条件，或者时间很长了
	if messageCondition && timeCondition {
		return true
	}

	// 如果消息很多，即使时间不够也要触发
	if state.MessagesSinceLastSummary >= stm.config.MessageThreshold*2 && minTimeGap {
		return true
	}

	// 如果时间很长，即使消息不多也要触发
	if timeSinceLastSummary.Hours() >= 1 && state.MessagesSinceLastSummary >= 2 {
		return true
	}

	return false
}

// MarkSummaryUpdated 标记摘要已更新
func (stm *SummaryTriggerManager) MarkSummaryUpdated(sessionKey string) {
	stm.mutex.Lock()
	defer stm.mutex.Unlock()

	if state, exists := stm.sessionStates[sessionKey]; exists {
		state.LastSummaryTime = time.Now()
		state.MessagesSinceLastSummary = 0
	}
}

// GetSessionState 获取会话状态（用于调试）
func (stm *SummaryTriggerManager) GetSessionState(sessionKey string) *SessionState {
	stm.mutex.RLock()
	defer stm.mutex.RUnlock()

	if state, exists := stm.sessionStates[sessionKey]; exists {
		// 返回副本避免并发问题
		return &SessionState{
			LastSummaryTime:          state.LastSummaryTime,
			MessagesSinceLastSummary: state.MessagesSinceLastSummary,
			TotalMessages:            state.TotalMessages,
		}
	}
	return nil
}

// CleanupOldSessions 清理旧会话状态（建议定期调用）
func (stm *SummaryTriggerManager) CleanupOldSessions(maxAge time.Duration) {
	stm.mutex.Lock()
	defer stm.mutex.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for sessionKey, state := range stm.sessionStates {
		if state.LastSummaryTime.Before(cutoff) {
			delete(stm.sessionStates, sessionKey)
		}
	}
}

// generateSessionKey 生成会话键
func generateSessionKey(userID, sessionID string) string {
	return userID + ":" + sessionID
}
