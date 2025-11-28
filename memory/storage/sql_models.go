package storage

import (
	"encoding/json"
	"time"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/schema"
)

// UserMemoryModel GORM模型 - 用户记忆表
type UserMemoryModel struct {
	ID        string    `gorm:"primaryKey;size:255" json:"id"`
	UserID    string    `gorm:"index;size:255;not null" json:"userId"`
	Memory    string    `gorm:"type:text;not null" json:"memory"`
	Input     string    `gorm:"type:text" json:"input"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

// SessionSummaryModel GORM模型 - 会话摘要表
type SessionSummaryModel struct {
	SessionID string    `gorm:"primaryKey;size:255" json:"sessionId"`
	UserID    string    `gorm:"primaryKey;size:255" json:"userId"`
	Summary   string    `gorm:"type:text;not null" json:"summary"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

// ConversationMessageModel GORM模型 - 对话消息表
type ConversationMessageModel struct {
	ID        string `gorm:"primaryKey;size:255" json:"id"`
	SessionID string `gorm:"index;size:255;not null" json:"sessionId"`
	UserID    string `gorm:"index;size:255;not null" json:"userId"`
	Role      string `gorm:"size:50;not null" json:"role"`
	// 保留Content字段用于向后兼容
	Content string `gorm:"type:text" json:"content,omitempty"`
	// 多部分内容，以JSON字符串形式存储（使用text类型兼容更多数据库）
	PartsJSON string                    `gorm:"type:text" json:"-"`       // 不直接暴露
	Parts     []schema.MessageInputPart `gorm:"-" json:"parts,omitempty"` // 用于业务逻辑
	CreatedAt time.Time                 `gorm:"autoCreateTime" json:"createdAt"`
}

// 模型转换函数

// ToUserMemory 将数据库模型转换为业务模型
func (m *UserMemoryModel) ToUserMemory() *memory.UserMemory {
	userMemory := &memory.UserMemory{
		ID:        m.ID,
		UserID:    m.UserID,
		Memory:    m.Memory,
		Input:     m.Input,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}

	return userMemory
}

// FromUserMemory 将业务模型转换为数据库模型
func (m *UserMemoryModel) FromUserMemory(userMemory *memory.UserMemory) {
	m.ID = userMemory.ID
	m.UserID = userMemory.UserID
	m.Memory = userMemory.Memory
	m.Input = userMemory.Input
	m.CreatedAt = userMemory.CreatedAt
	m.UpdatedAt = userMemory.UpdatedAt
}

// ToSessionSummary 将数据库模型转换为业务模型
func (m *SessionSummaryModel) ToSessionSummary() *memory.SessionSummary {
	sessionSummary := &memory.SessionSummary{
		SessionID: m.SessionID,
		UserID:    m.UserID,
		Summary:   m.Summary,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}

	return sessionSummary
}

// FromSessionSummary 将业务模型转换为数据库模型
func (m *SessionSummaryModel) FromSessionSummary(sessionSummary *memory.SessionSummary) {
	m.SessionID = sessionSummary.SessionID
	m.UserID = sessionSummary.UserID
	m.Summary = sessionSummary.Summary
	m.CreatedAt = sessionSummary.CreatedAt
	m.UpdatedAt = sessionSummary.UpdatedAt
}

// ToConversationMessage 将数据库模型转换为业务模型
func (m *ConversationMessageModel) ToConversationMessage() *memory.ConversationMessage {
	var parts []schema.MessageInputPart
	content := m.Content

	// 检查PartsJSON字段
	if m.PartsJSON != "" {
		// 有多部分内容，反序列化Parts
		if err := json.Unmarshal([]byte(m.PartsJSON), &parts); err != nil {
			// 反序列化失败，使用空数组
			parts = []schema.MessageInputPart{}
		}
		content = "" // 有Parts时清空Content，避免重复
	} else {
		// 没有多部分内容，使用Content字段
		parts = []schema.MessageInputPart{} // 空的Parts数组
	}

	return &memory.ConversationMessage{
		ID:        m.ID,
		SessionID: m.SessionID,
		UserID:    m.UserID,
		Role:      m.Role,
		Content:   content,
		Parts:     parts,
		CreatedAt: m.CreatedAt,
	}
}

// FromConversationMessage 将业务模型转换为数据库模型
func (m *ConversationMessageModel) FromConversationMessage(message *memory.ConversationMessage) {
	m.ID = message.ID
	m.SessionID = message.SessionID
	m.UserID = message.UserID
	m.Role = message.Role

	// 根据消息内容决定存储方式：
	// 1. 如果有多部分内容（Parts），优先使用PartsJSON存储，Content为空
	// 2. 如果只有简单文本内容，使用Content存储，PartsJSON为null
	if len(message.Parts) > 0 {
		// 有多部分内容，序列化Parts为JSON
		m.Content = "" // 清空Content，避免数据冗余
		if partsBytes, err := json.Marshal(message.Parts); err != nil {
			// 序列化失败，设置为null
			m.PartsJSON = ""
		} else {
			m.PartsJSON = string(partsBytes)
		}
	} else {
		// 只有简单文本内容，使用Content字段
		m.Content = message.Content
		m.PartsJSON = "" // 表示没有多部分内容
	}

	m.CreatedAt = message.CreatedAt
}
