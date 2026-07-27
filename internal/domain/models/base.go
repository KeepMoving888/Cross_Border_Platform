package models

import (
	"time"

	"gorm.io/gorm"
)

// BaseModel 基础模型,所有表共用
type BaseModel struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	// 让 GORM 自动管理时间(避免 MySQL 8.0 sql_mode 严格模式下的 default 冲突)
	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"index" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// SoftDeleteModel 软删除基础模型
type SoftDeleteModel struct {
	BaseModel
	DeletedBy string `gorm:"size:64" json:"deleted_by,omitempty"`
}

// Pagination 分页参数
type Pagination struct {
	Page     int   `form:"page" json:"page"`
	PageSize int   `form:"page_size" json:"page_size"`
}

func (p *Pagination) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 || p.PageSize > 200 {
		p.PageSize = 20
	}
}

func (p *Pagination) Offset() int {
	return (p.Page - 1) * p.PageSize
}

// TimeRange 时间范围查询
type TimeRange struct {
	StartTime *time.Time `form:"start_time" json:"start_time,omitempty"`
	EndTime   *time.Time `form:"end_time" json:"end_time,omitempty"`
}

// CommonStatus 通用状态(0=禁用 1=启用)
type CommonStatus uint8

const (
	StatusDisabled CommonStatus = 0
	StatusEnabled  CommonStatus = 1
)
