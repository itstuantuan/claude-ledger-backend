package auth

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	StoreID      uuid.UUID `gorm:"type:uuid;not null"`
	Account      string
	PasswordHash string
	Name         string
	Phone        string
	Status       string
	AuthVersion  int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Role         string   `gorm:"-"`
	Permissions  []string `gorm:"-"`
}

type RefreshSession struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	FamilyID     uuid.UUID `gorm:"type:uuid;not null"`
	StoreID      uuid.UUID `gorm:"type:uuid;not null"`
	UserID       uuid.UUID `gorm:"type:uuid;not null"`
	TokenHash    []byte
	ExpiresAt    time.Time
	RevokedAt    *time.Time
	LastUsedAt   *time.Time
	ReplacedByID *uuid.UUID
	UserAgent    string
	IP           string `gorm:"type:inet"`
	CreatedAt    time.Time
}

func (User) TableName() string           { return "users" }
func (RefreshSession) TableName() string { return "refresh_sessions" }
