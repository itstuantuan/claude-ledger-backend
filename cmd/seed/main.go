package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"cloud-ledger-backend/internal/platform/config"
	"cloud-ledger-backend/internal/platform/database"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var grants = map[string][]string{
	"OWNER":   {"workers:read", "workers:write", "materials:read", "materials:write", "orders:create", "returns:request", "returns:confirm", "returns:manual", "finance:read", "payments:create", "prepaid:deposit", "ledger:adjust", "reconciliation:read", "reports:read", "system:manage"},
	"FINANCE": {"workers:read", "materials:read", "finance:read", "payments:create", "prepaid:deposit", "reconciliation:read", "reports:read"},
	"CLERK":   {"workers:read", "materials:read", "orders:create", "returns:request"},
}

func main() {
	if os.Getenv("APP_ENV") == "production" {
		panic("development seed is disabled in production")
	}
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	db, sqlDB, err := database.Open(cfg.Database)
	if err != nil {
		panic(err)
	}
	defer sqlDB.Close()
	if err := seed(context.Background(), db); err != nil {
		panic(err)
	}
	fmt.Println("development auth seed completed; password: Paint123!")
}

func seed(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		storeID := uuid.MustParse("00000000-0000-7000-8000-000000000001")
		if err := tx.Exec(`INSERT INTO stores (id,name,phone,address,timezone,status) VALUES (?,?,?,?,?,?) ON CONFLICT (id) DO NOTHING`, storeID, "云记账示范门店", "", "", "Asia/Shanghai", "ACTIVE").Error; err != nil {
			return err
		}
		password, err := bcrypt.GenerateFromPassword([]byte("Paint123!"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		accounts := []struct{ account, name, role string }{{"owner", "门店老板", "OWNER"}, {"finance", "门店财务", "FINANCE"}, {"clerk", "门店店员", "CLERK"}}
		for _, item := range accounts {
			roleID := uuid.NewSHA1(storeID, []byte("role:"+item.role))
			userID := uuid.NewSHA1(storeID, []byte("user:"+item.account))
			if err := tx.Exec(`INSERT INTO roles (id,store_id,code,name) VALUES (?,?,?,?) ON CONFLICT (store_id,code) DO NOTHING`, roleID, storeID, item.role, item.role).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO users (id,store_id,account,password_hash,name,status,auth_version,created_at,updated_at) VALUES (?,?,?,?,?,'ACTIVE',1,?,?) ON CONFLICT (store_id,account) DO UPDATE SET name=EXCLUDED.name`, userID, storeID, item.account, string(password), item.name, time.Now().UTC(), time.Now().UTC()).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO user_roles (user_id,role_id) VALUES (?,?) ON CONFLICT DO NOTHING`, userID, roleID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO role_permissions (role_id,permission_id) SELECT ?,id FROM permissions WHERE code IN ? ON CONFLICT DO NOTHING`, roleID, grants[item.role]).Error; err != nil {
				return err
			}
		}
		return seedMasterData(tx, storeID, uuid.NewSHA1(storeID, []byte("user:owner")))
	})
}
