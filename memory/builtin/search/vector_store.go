package search

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

type GormVectorStore struct {
	db        *gorm.DB
	tableName string
}

type gormVectorRecord struct {
	ID           string    `gorm:"column:id"`
	SessionID    string    `gorm:"column:session_id"`
	UserID       string    `gorm:"column:user_id"`
	Role         string    `gorm:"column:role"`
	Content      string    `gorm:"column:content"`
	Parts        string    `gorm:"column:parts"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	Embedding    []byte    `gorm:"column:embedding"`
	EmbeddingDim int       `gorm:"column:embedding_dim"`
}

func NewGormVectorStore(db *gorm.DB, tableName string) (*GormVectorStore, error) {
	if db == nil {
		return nil, errors.New("db is required")
	}
	if strings.TrimSpace(tableName) == "" {
		return nil, errors.New("table name is required")
	}
	return &GormVectorStore{db: db, tableName: tableName}, nil
}

func (s *GormVectorStore) Upsert(ctx context.Context, msg *Message, vector []float64) error {
	if s == nil {
		return errors.New("gorm vector store is nil")
	}
	if msg == nil || strings.TrimSpace(msg.ID) == "" {
		return errors.New("message id is required")
	}
	if len(vector) == 0 {
		return errors.New("vector is empty")
	}

	blob, err := encodeVector(vector)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).
		Table(s.tableName).
		Where("id = ?", msg.ID).
		Updates(map[string]any{
			"embedding":     blob,
			"embedding_dim": len(vector),
		}).Error
}

func (s *GormVectorStore) Search(ctx context.Context, q *SearchQuery, vector []float64, limit int) ([]*SearchHit, error) {
	if s == nil {
		return nil, errors.New("gorm vector store is nil")
	}
	if q == nil {
		return nil, errors.New("search query is nil")
	}
	if len(vector) == 0 {
		return nil, errors.New("query vector is empty")
	}

	var records []gormVectorRecord
	query := s.db.WithContext(ctx).
		Table(s.tableName).
		Select("id, session_id, user_id, role, content, parts, created_at, embedding, embedding_dim").
		Where("session_id = ? AND user_id = ?", q.SessionID, q.UserID).
		Where("embedding IS NOT NULL AND embedding_dim > 0")

	if role := strings.TrimSpace(q.Role); role != "" {
		query = query.Where("role = ?", role)
	}
	if q.Since != nil {
		query = query.Where("created_at >= ?", q.Since)
	}
	if q.Until != nil {
		query = query.Where("created_at <= ?", q.Until)
	}

	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("search vectors: %w", err)
	}

	hits := make([]*SearchHit, 0, len(records))
	for _, record := range records {
		candidate, err := decodeVector(record.Embedding)
		if err != nil || len(candidate) == 0 || len(candidate) != len(vector) {
			continue
		}

		parts := decodeParts(record.Parts)
		score := cosineSimilarity(vector, candidate)
		hits = append(hits, &SearchHit{
			Message: &Message{
				ID:        record.ID,
				SessionID: record.SessionID,
				UserID:    record.UserID,
				Role:      record.Role,
				Content:   record.Content,
				Parts:     parts,
				CreatedAt: record.CreatedAt,
			},
			Score: score,
		})
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score == hits[j].Score {
			return hits[i].Message.CreatedAt.After(hits[j].Message.CreatedAt)
		}
		return hits[i].Score > hits[j].Score
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func encodeVector(vector []float64) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, len(vector)*8))
	for _, value := range vector {
		if err := binary.Write(buf, binary.LittleEndian, value); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func decodeVector(blob []byte) ([]float64, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	if len(blob)%8 != 0 {
		return nil, errors.New("invalid vector blob length")
	}

	vector := make([]float64, 0, len(blob)/8)
	reader := bytes.NewReader(blob)
	for reader.Len() > 0 {
		var value float64
		if err := binary.Read(reader, binary.LittleEndian, &value); err != nil {
			return nil, err
		}
		vector = append(vector, value)
	}
	return vector, nil
}

func decodeParts(raw string) []schema.MessageInputPart {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var parts []schema.MessageInputPart
	if err := json.Unmarshal([]byte(raw), &parts); err != nil {
		return nil
	}
	return parts
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}

	var dot float64
	var normA float64
	var normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
