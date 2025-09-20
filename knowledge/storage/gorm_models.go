package storage

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// DocumentModel GORM 文档模型。向量数据由 VectorDB 存储和管理
type DocumentModel struct {
	ID        string         `gorm:"primaryKey;type:varchar(255)" json:"id"`
	Content   string         `gorm:"type:text" json:"content"`
	Metadata  string         `gorm:"type:text" json:"metadata"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// SetMetadata 设置元数据
func (d *DocumentModel) SetMetadata(metadata map[string]interface{}) error {
	if metadata == nil {
		d.Metadata = ""
		return nil
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	d.Metadata = string(data)
	return nil
}

// GetMetadata 获取元数据
func (d *DocumentModel) GetMetadata() (map[string]interface{}, error) {
	if d.Metadata == "" {
		return make(map[string]interface{}), nil
	}

	var metadata map[string]interface{}
	err := json.Unmarshal([]byte(d.Metadata), &metadata)
	return metadata, err
}
