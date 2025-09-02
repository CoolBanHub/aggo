package storage

import (
	"time"

	"github.com/CoolBanHub/aggo/memory"
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

// TableName 指定表名
func (UserMemoryModel) TableName() string {
	return "aggo_mem_user_memories"
}

// SessionSummaryModel GORM模型 - 会话摘要表
type SessionSummaryModel struct {
	SessionID string    `gorm:"primaryKey;size:255" json:"sessionId"`
	UserID    string    `gorm:"primaryKey;size:255" json:"userId"`
	Summary   string    `gorm:"type:text;not null" json:"summary"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updatedAt"`
}

// TableName 指定表名
func (SessionSummaryModel) TableName() string {
	return "aggo_mem_session_summaries"
}

// ConversationMessageModel GORM模型 - 对话消息表
type ConversationMessageModel struct {
	ID        string    `gorm:"primaryKey;size:255" json:"id"`
	SessionID string    `gorm:"index;size:255;not null" json:"sessionId"`
	UserID    string    `gorm:"index;size:255;not null" json:"userId"`
	Role      string    `gorm:"size:50;not null" json:"role"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

// TableName 指定表名
func (ConversationMessageModel) TableName() string {
	return "aggo_mem_conversation_messages"
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
	return &memory.ConversationMessage{
		ID:        m.ID,
		SessionID: m.SessionID,
		UserID:    m.UserID,
		Role:      m.Role,
		Content:   m.Content,
		CreatedAt: m.CreatedAt,
	}
}

// FromConversationMessage 将业务模型转换为数据库模型
func (m *ConversationMessageModel) FromConversationMessage(message *memory.ConversationMessage) {
	m.ID = message.ID
	m.SessionID = message.SessionID
	m.UserID = message.UserID
	m.Role = message.Role
	m.Content = message.Content
	m.CreatedAt = message.CreatedAt
}
