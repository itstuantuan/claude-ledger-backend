package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrSessionInvalid = errors.New("refresh session is invalid")

type Repository interface {
	FindUserByAccount(context.Context, string) (User, error)
	FindUserByID(context.Context, uuid.UUID) (User, error)
	CreateSession(context.Context, RefreshSession) error
	RotateSession(context.Context, []byte, RefreshSession, time.Time) (User, error)
	RevokeSession(context.Context, []byte, time.Time) error
}

type GormRepository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *GormRepository { return &GormRepository{db: db} }

func (r *GormRepository) FindUserByAccount(ctx context.Context, account string) (User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("account = ?", account).First(&user).Error; err != nil {
		return User{}, err
	}
	return r.loadGrants(ctx, r.db, user)
}

func (r *GormRepository) FindUserByID(ctx context.Context, id uuid.UUID) (User, error) {
	var user User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return User{}, err
	}
	return r.loadGrants(ctx, r.db, user)
}

func (r *GormRepository) loadGrants(ctx context.Context, db *gorm.DB, user User) (User, error) {
	var roles []string
	if err := db.WithContext(ctx).Table("roles r").Select("r.code").Joins("JOIN user_roles ur ON ur.role_id = r.id").Where("ur.user_id = ?", user.ID).Order("r.code").Scan(&roles).Error; err != nil {
		return User{}, err
	}
	if len(roles) > 0 {
		user.Role = roles[0]
	}
	if err := db.WithContext(ctx).Table("permissions p").Distinct("p.code").Joins("JOIN role_permissions rp ON rp.permission_id = p.id").Joins("JOIN user_roles ur ON ur.role_id = rp.role_id").Where("ur.user_id = ?", user.ID).Order("p.code").Pluck("p.code", &user.Permissions).Error; err != nil {
		return User{}, err
	}
	if user.Permissions == nil {
		user.Permissions = []string{}
	}
	return user, nil
}

func (r *GormRepository) CreateSession(ctx context.Context, session RefreshSession) error {
	return r.db.WithContext(ctx).Create(&session).Error
}

func (r *GormRepository) RotateSession(ctx context.Context, oldHash []byte, replacement RefreshSession, now time.Time) (User, error) {
	var user User
	invalid := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current RefreshSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", oldHash).First(&current).Error; err != nil {
			return ErrSessionInvalid
		}
		if current.RevokedAt != nil || !current.ExpiresAt.After(now) {
			invalid = true
			if current.RevokedAt != nil {
				return tx.Model(&RefreshSession{}).Where("family_id = ? AND revoked_at IS NULL", current.FamilyID).Updates(map[string]any{"revoked_at": now, "last_used_at": now}).Error
			}
			return nil
		}
		if err := tx.Where("id = ? AND store_id = ?", current.UserID, current.StoreID).First(&user).Error; err != nil {
			return ErrSessionInvalid
		}
		if user.Status != "ACTIVE" {
			return ErrSessionInvalid
		}
		replacement.UserID, replacement.StoreID, replacement.FamilyID = user.ID, user.StoreID, current.FamilyID
		if err := tx.Create(&replacement).Error; err != nil {
			return err
		}
		if err := tx.Model(&RefreshSession{}).Where("id = ? AND revoked_at IS NULL", current.ID).Updates(map[string]any{"revoked_at": now, "last_used_at": now, "replaced_by_id": replacement.ID}).Error; err != nil {
			return err
		}
		var err error
		user, err = r.loadGrants(ctx, tx, user)
		return err
	})
	if err == nil && invalid {
		return User{}, ErrSessionInvalid
	}
	return user, err
}

func (r *GormRepository) RevokeSession(ctx context.Context, hash []byte, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session RefreshSession
		if err := tx.Where("token_hash = ?", hash).First(&session).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		return tx.Model(&RefreshSession{}).Where("family_id = ? AND revoked_at IS NULL", session.FamilyID).Updates(map[string]any{"revoked_at": now, "last_used_at": now}).Error
	})
}
