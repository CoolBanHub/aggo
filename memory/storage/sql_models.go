package storage

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/CoolBanHub/aggo/memory"
	"github.com/cloudwego/eino/schema"
)

// MessageParts 自定义 GORM 类型，用于处理 []schema.MessageInputPart 的序列化
type MessageParts []schema.MessageInputPart

// Value 实现 driver.Valuer 接口，用于将数据存入数据库
func (mp MessageParts) Value() (driver.Value, error) {
	if mp == nil {
		return nil, nil
	}
	return json.Marshal(mp)
}

// Scan 实现 sql.Scanner 接口，用于从数据库读取数据
func (mp *MessageParts) Scan(value interface{}) error {
	if value == nil {
		*mp = MessageParts{}
		return nil
	}

	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, mp)
	case string:
		return json.Unmarshal([]byte(v), mp)
	default:
		return nil
	}
}

// GormValue 为 GORM 提供特定的数据类型支持
func (mp MessageParts) GormDataType() string {
	return "text"
}

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
	// 多部分内容，使用自定义类型直接存储
	Parts     MessageParts `gorm:"type:text" json:"parts,omitempty"`
	CreatedAt time.Time    `gorm:"autoCreateTime" json:"createdAt"`
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
	// Parts 现在是自定义类型，可以直接转换为 []schema.MessageInputPart
	parts := []schema.MessageInputPart(m.Parts)
	content := m.Content

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
	m.Content = message.Content
	m.Parts = message.Parts
	m.CreatedAt = message.CreatedAt
}
