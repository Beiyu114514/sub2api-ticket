package api

import (
	"context"
	"errors"
	"log"
	"time"

	"sub2api-ticket/internal/model"
	"sub2api-ticket/internal/storage"
)

// CleanOrphanUploads 定期清理孤儿上传：上传后 24h 仍 ticket_id=0 且无任何消息引用
// → 删除存储对象 + 删除 Upload 记录。每小时跑一次，每轮最多 100 个。
func (s *UploadService) CleanOrphanUploads() {
	ticker := time.NewTicker(time.Hour)
	go func() {
		defer ticker.Stop()
		for range ticker.C {
			s.cleanOrphanOnce()
		}
	}()
}

// orphanGrace 上传后允许挂单的宽限期（覆盖建单/回复的正常流程时长）
const orphanGrace = 24 * time.Hour

func (s *UploadService) cleanOrphanOnce() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[cleanup] panic: %v", r)
		}
	}()
	var orphans []model.Upload
	if err := s.G.Where("ticket_id = 0 AND created_at < ?", time.Now().Add(-orphanGrace)).
		Limit(100).Find(&orphans).Error; err != nil {
		return
	}
	deleted := 0
	for _, o := range orphans {
		// 仍被消息引用则修正 ticket_id 而非删除（防御 Rebind 漏挂）
		var cnt int64
		if err := s.G.Model(&model.TicketMessage{}).
			Where("attachments LIKE ?", "%"+o.Key+"%").Count(&cnt).Error; err == nil && cnt > 0 {
			var msg model.TicketMessage
			if s.G.Where("attachments LIKE ?", "%"+o.Key+"%").First(&msg).Error == nil {
				s.G.Model(&o).Update("ticket_id", msg.TicketID)
			}
			continue
		}
		if err := s.DeleteByKey(o.Key); err != nil && !errors.Is(err, storage.ErrNotFound) {
			log.Printf("[cleanup] 删除对象失败 %s: %v", o.Key, err)
			continue // 对象删不掉就保留记录，下轮重试
		}
		if err := s.G.Delete(&o).Error; err == nil {
			deleted++
		}
	}
	if deleted > 0 {
		log.Printf("[cleanup] 清理孤儿上传 %d 个", deleted)
	}
}

// DeleteByKey 按 key 删除对象（按当前配置构造后端；若管理员切换过存储模式且
// 未迁移数据会删错后端而失败，失败保留记录重试，无数据损坏风险）
func (s *UploadService) DeleteByKey(key string) error {
	return s.backend().Delete(context.Background(), key)
}
