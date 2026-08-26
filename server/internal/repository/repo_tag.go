package repository

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/server-probe/server/internal/model"
)

// ErrTagNotFound 标签不存在
var ErrTagNotFound = errors.New("标签不存在")

// ErrTagAlreadyExists 同名标签已存在
var ErrTagAlreadyExists = errors.New("同名标签已存在")

// TagRepository 标签 CRUD
type TagRepository struct {
	db *gorm.DB
}

// NewTagRepository 创建标签 repository
func NewTagRepository(db *gorm.DB) *TagRepository {
	return &TagRepository{db: db}
}

// Create 创建标签
func (r *TagRepository) Create(tag *model.Tag) error {
	err := r.db.Create(tag).Error
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrTagAlreadyExists
	}
	return err
}

// GetByID 根据 ID 获取标签
func (r *TagRepository) GetByID(id int64) (*model.Tag, error) {
	var tag model.Tag
	if err := r.db.First(&tag, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTagNotFound
		}
		return nil, err
	}
	return &tag, nil
}

// List 获取所有标签
func (r *TagRepository) List() ([]model.Tag, error) {
	var tags []model.Tag
	if err := r.db.Order("id ASC").Find(&tags).Error; err != nil {
		return nil, err
	}
	return tags, nil
}

// Update 更新标签
func (r *TagRepository) Update(tag *model.Tag) error {
	err := r.db.Save(tag).Error
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrTagAlreadyExists
	}
	return err
}

// Delete 删除标签
func (r *TagRepository) Delete(id int64) error {
	return r.db.Delete(&model.Tag{}, id).Error
}
