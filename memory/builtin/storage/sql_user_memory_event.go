package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/CoolBanHub/aggo/memory/builtin"
	"github.com/CoolBanHub/aggo/utils"
)

// SaveUserMemoryEvent 保存一条用户记忆事件
func (s *SQLStore) SaveUserMemoryEvent(ctx context.Context, event *builtin.UserMemoryEvent) error {
	if event == nil {
		return errors.New("事件对象不能为空")
	}
	if event.UserID == "" {
		return errors.New("用户ID不能为空")
	}
	if strings.TrimSpace(event.Summary) == "" {
		return errors.New("事件内容不能为空")
	}

	if event.ID == "" {
		event.ID = utils.GetULID()
	}
	now := time.Now()
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	if event.EventDate.IsZero() {
		event.EventDate = now
	}
	if event.Type == "" {
		event.Type = builtin.UserMemoryEventTypeEvent
	}

	model := &UserMemoryEventModel{}
	model.FromUserMemoryEvent(event)

	if err := s.db.WithContext(ctx).
		Table(s.tableNameProvider.GetUserMemoryEventTableName()).
		Create(model).Error; err != nil {
		return fmt.Errorf("保存用户记忆事件失败: %v", err)
	}
	return nil
}

// ListRecentUserMemoryEvents 返回用户最近的事件，按 EventDate 倒序
func (s *SQLStore) ListRecentUserMemoryEvents(ctx context.Context, userID string, limit int) ([]*builtin.UserMemoryEvent, error) {
	if userID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	var rows []UserMemoryEventModel
	q := s.db.WithContext(ctx).
		Table(s.tableNameProvider.GetUserMemoryEventTableName()).
		Where("user_id = ?", userID).
		Order("event_date DESC, created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询最近用户记忆事件失败: %v", err)
	}

	out := make([]*builtin.UserMemoryEvent, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].ToUserMemoryEvent())
	}
	return out, nil
}

// SearchUserMemoryEvents 按查询条件检索用户事件
func (s *SQLStore) SearchUserMemoryEvents(ctx context.Context, query *builtin.UserMemoryEventQuery) ([]*builtin.UserMemoryEvent, error) {
	if query == nil || query.UserID == "" {
		return nil, errors.New("用户ID不能为空")
	}

	q := s.db.WithContext(ctx).
		Table(s.tableNameProvider.GetUserMemoryEventTableName()).
		Where("user_id = ?", query.UserID)

	if query.Type != "" {
		q = q.Where("type = ?", query.Type)
	}
	if query.Since != nil {
		q = q.Where("event_date >= ?", *query.Since)
	}
	if query.Until != nil {
		q = q.Where("event_date <= ?", *query.Until)
	}

	// 关键词在 summary 和 keywords JSON 文本里做 LIKE 匹配。
	// 关键词数量通常很少（<=10），逐个条件累加在大多数后端可命中索引前缀过滤。
	keywords := dedupNonEmpty(query.Keywords)
	match := strings.ToLower(strings.TrimSpace(query.Match))
	if match != "all" {
		match = "any"
	}
	if len(keywords) > 0 {
		clauseParts := make([]string, 0, len(keywords))
		args := make([]any, 0, len(keywords)*2)
		for _, kw := range keywords {
			like := "%" + kw + "%"
			clauseParts = append(clauseParts, "(summary LIKE ? OR keywords LIKE ?)")
			args = append(args, like, like)
		}
		sep := " OR "
		if match == "all" {
			sep = " AND "
		}
		q = q.Where(strings.Join(clauseParts, sep), args...)
	}

	q = q.Order("event_date DESC, created_at DESC")
	if query.Limit > 0 {
		q = q.Limit(query.Limit)
	}

	var rows []UserMemoryEventModel
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("检索用户记忆事件失败: %v", err)
	}

	out := make([]*builtin.UserMemoryEvent, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].ToUserMemoryEvent())
	}
	return out, nil
}

// DeleteUserMemoryEvent 删除指定事件
func (s *SQLStore) DeleteUserMemoryEvent(ctx context.Context, userID, eventID string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if eventID == "" {
		return errors.New("事件ID不能为空")
	}
	if err := s.db.WithContext(ctx).
		Table(s.tableNameProvider.GetUserMemoryEventTableName()).
		Where("user_id = ? AND id = ?", userID, eventID).
		Delete(&UserMemoryEventModel{}).Error; err != nil {
		return fmt.Errorf("删除用户记忆事件失败: %v", err)
	}
	return nil
}

// ClearUserMemoryEvents 清空用户的事件
func (s *SQLStore) ClearUserMemoryEvents(ctx context.Context, userID string) error {
	if userID == "" {
		return errors.New("用户ID不能为空")
	}
	if err := s.db.WithContext(ctx).
		Table(s.tableNameProvider.GetUserMemoryEventTableName()).
		Where("user_id = ?", userID).
		Delete(&UserMemoryEventModel{}).Error; err != nil {
		return fmt.Errorf("清空用户记忆事件失败: %v", err)
	}
	return nil
}

func dedupNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}
