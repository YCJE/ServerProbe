package repository

import (
	crand "crypto/rand"
	"math/big"
	"time"

	"github.com/server-probe/server/internal/model"
)

// 注意: SharePageRepository 的 struct、构造函数、Create、GetByShareID、List、Delete
// 已在 repo_record.go 中声明，此文件补充缺失的方法。

// GetByID 根据 ID 获取分享页
func (r *SharePageRepository) GetByID(id int64) (*model.SharePage, error) {
	var page model.SharePage
	if err := r.db.First(&page, id).Error; err != nil {
		return nil, err
	}
	return &page, nil
}

// ListEnabled 获取已启用的分享页
func (r *SharePageRepository) ListEnabled() ([]model.SharePage, error) {
	var pages []model.SharePage
	if err := r.db.Where("enabled = ?", true).Order("sort_order ASC, id ASC").Find(&pages).Error; err != nil {
		return nil, err
	}
	return pages, nil
}

// Update 更新分享页
func (r *SharePageRepository) Update(page *model.SharePage) error {
	return r.db.Save(page).Error
}

// UpdateEnabled 使用 Select 强制更新 enabled 字段，避免 GORM default tag 导致零值被忽略
func (r *SharePageRepository) UpdateEnabled(page *model.SharePage, enabled bool) error {
	return r.db.Model(page).Select("enabled").Update("enabled", enabled).Error
}

// GenerateShareID 生成 8 字符随机字符串作为 share_id
func GenerateShareID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	for i := range b {
		idx, err := crand.Int(crand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// 极端情况下回退到基于时间的值
			idx = big.NewInt(time.Now().UnixNano() % int64(len(charset)))
		}
		b[i] = charset[idx.Int64()]
	}
	return string(b)
}
