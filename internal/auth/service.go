package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"cloud-ledger-backend/internal/platform/apperror"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type Service struct {
	repo           Repository
	tokens         TokenManager
	refreshExpires time.Duration
	now            func() time.Time
}

func NewService(repo Repository, tokens TokenManager, refreshExpires time.Duration) *Service {
	return &Service{repo: repo, tokens: tokens, refreshExpires: refreshExpires, now: time.Now}
}

func (s *Service) Login(ctx context.Context, request LoginRequest, userAgent, ip string) (SessionResponse, string, error) {
	user, err := s.repo.FindUserByAccount(ctx, strings.TrimSpace(request.Account))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(request.Password)) != nil {
		return SessionResponse{}, "", apperror.New(401, "INVALID_CREDENTIALS", "账号或密码不正确。")
	}
	if user.Status != "ACTIVE" {
		return SessionResponse{}, "", apperror.New(403, "ACCOUNT_DISABLED", "该账号已停用，请联系管理员。")
	}
	return s.issue(ctx, user, userAgent, ip)
}

func (s *Service) Refresh(ctx context.Context, currentToken, userAgent, ip string) (SessionResponse, string, error) {
	if currentToken == "" {
		return SessionResponse{}, "", apperror.New(401, "SESSION_EXPIRED", "登录已过期，请重新登录。")
	}
	raw, hash, err := newRefreshToken()
	if err != nil {
		return SessionResponse{}, "", err
	}
	now := s.now().UTC()
	replacement := RefreshSession{ID: uuid.New(), TokenHash: hash, ExpiresAt: now.Add(s.refreshExpires), UserAgent: userAgent, IP: ip, CreatedAt: now}
	user, err := s.repo.RotateSession(ctx, hashRefreshToken(currentToken), replacement, now)
	if err != nil {
		return SessionResponse{}, "", apperror.New(401, "SESSION_EXPIRED", "登录已过期，请重新登录。")
	}
	access, expires, err := s.tokens.Access(user, now)
	if err != nil {
		return SessionResponse{}, "", err
	}
	return SessionResponse{AccessToken: access, ExpiresIn: expires, User: userResponse(user)}, raw, nil
}

func (s *Service) Logout(ctx context.Context, currentToken string) error {
	if currentToken == "" {
		return nil
	}
	return s.repo.RevokeSession(ctx, hashRefreshToken(currentToken), s.now().UTC())
}

func (s *Service) Me(ctx context.Context, id uuid.UUID) (UserResponse, error) {
	user, err := s.repo.FindUserByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return UserResponse{}, apperror.New(401, "UNAUTHENTICATED", "请先登录。")
	}
	if err != nil {
		return UserResponse{}, err
	}
	if user.Status != "ACTIVE" {
		return UserResponse{}, apperror.New(403, "ACCOUNT_DISABLED", "该账号已停用，请联系管理员。")
	}
	return userResponse(user), nil
}

func (s *Service) issue(ctx context.Context, user User, userAgent, ip string) (SessionResponse, string, error) {
	now := s.now().UTC()
	access, expires, err := s.tokens.Access(user, now)
	if err != nil {
		return SessionResponse{}, "", err
	}
	raw, hash, err := newRefreshToken()
	if err != nil {
		return SessionResponse{}, "", err
	}
	id := uuid.New()
	session := RefreshSession{ID: id, FamilyID: id, StoreID: user.StoreID, UserID: user.ID, TokenHash: hash, ExpiresAt: now.Add(s.refreshExpires), UserAgent: userAgent, IP: ip, CreatedAt: now}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		return SessionResponse{}, "", err
	}
	return SessionResponse{AccessToken: access, ExpiresIn: expires, User: userResponse(user)}, raw, nil
}
