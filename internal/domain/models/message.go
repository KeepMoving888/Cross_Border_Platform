package models

// Message 站内消息/通知
type Message struct {
	BaseModel
	UserID  uint   `gorm:"index;not null" json:"user_id"`
	Type    string `gorm:"size:32;index;default:system" json:"type"` // system / stock_alert / purchase / finance / ai
	Level   string `gorm:"size:16;default:info" json:"level"`        // info / warning / critical / success
	Title   string `gorm:"size:255;not null" json:"title"`
	Content string `gorm:"type:text" json:"content"`
	RefType string `gorm:"size:32;index" json:"ref_type,omitempty"` // inventory / purchase / bill / product
	RefID   string `gorm:"size:64;index" json:"ref_id,omitempty"`
	Link    string `gorm:"size:255" json:"link,omitempty"`
	IsRead  bool   `gorm:"index;default:false" json:"is_read"`
}

func (Message) TableName() string { return "messages" }
