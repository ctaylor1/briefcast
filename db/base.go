package db

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base is
type Base struct {
	ID        string `sql:"type:uuid;primary_key"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time `gorm:"index"`
}

// BeforeCreate
func (base *Base) BeforeCreate(tx *gorm.DB) error {
	tx.Statement.SetColumn("ID", uuid.NewString())
	return nil
}
