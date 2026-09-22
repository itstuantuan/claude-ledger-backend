package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"cloud-ledger-backend/internal/platform/apperror"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type fakeRepository struct {
	user      User
	session   RefreshSession
	rotateErr error
	revoked   bool
}

func (f *fakeRepository) FindUserByAccount(context.Context, string) (User, error) { return f.user, nil }
func (f *fakeRepository) FindUserByID(context.Context, uuid.UUID) (User, error)   { return f.user, nil }
func (f *fakeRepository) CreateSession(_ context.Context, session RefreshSession) error {
	f.session = session
	return nil
}
func (f *fakeRepository) RotateSession(_ context.Context, _ []byte, session RefreshSession, _ time.Time) (User, error) {
	f.session = session
	return f.user, f.rotateErr
}
func (f *fakeRepository) RevokeSession(context.Context, []byte, time.Time) error {
	f.revoked = true
	return nil
}

func testUser(t *testing.T) User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("Paint123!"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return User{ID: uuid.New(), StoreID: uuid.New(), Account: "owner", PasswordHash: string(hash), Name: "门店老板", Status: "ACTIVE", AuthVersion: 1, Role: "OWNER", Permissions: []string{"system:manage"}}
}

func TestLoginMatchesFrontendSessionContract(t *testing.T) {
	repo := &fakeRepository{user: testUser(t)}
	service := NewService(repo, NewTokenManager("test-access-secret-at-least-32-bytes", 15*time.Minute), 7*24*time.Hour)
	session, refresh, err := service.Login(context.Background(), LoginRequest{Account: " owner ", Password: "Paint123!"}, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if session.User.Account != "owner" || session.User.Role != "OWNER" || session.ExpiresIn != 900 || session.AccessToken == "" || refresh == "" {
		t.Fatalf("unexpected session: %+v", session)
	}
	if string(repo.session.TokenHash) == refresh {
		t.Fatal("raw refresh token was persisted")
	}
}

func TestLoginRejectsBadPasswordAndDisabledUser(t *testing.T) {
	repo := &fakeRepository{user: testUser(t)}
	service := NewService(repo, NewTokenManager("test-access-secret-at-least-32-bytes", time.Minute), time.Hour)
	_, _, err := service.Login(context.Background(), LoginRequest{Account: "owner", Password: "wrong"}, "", "")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("unexpected error: %v", err)
	}
	repo.user.Status = "DISABLED"
	_, _, err = service.Login(context.Background(), LoginRequest{Account: "owner", Password: "Paint123!"}, "", "")
	if !errors.As(err, &appErr) || appErr.Code != "ACCOUNT_DISABLED" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRefreshRotatesAndLogoutRevokes(t *testing.T) {
	repo := &fakeRepository{user: testUser(t)}
	service := NewService(repo, NewTokenManager("test-access-secret-at-least-32-bytes", time.Minute), time.Hour)
	session, replacement, err := service.Refresh(context.Background(), "old-refresh", "agent", "127.0.0.1")
	if err != nil || session.AccessToken == "" || replacement == "" {
		t.Fatalf("refresh failed: %v", err)
	}
	if err := service.Logout(context.Background(), replacement); err != nil || !repo.revoked {
		t.Fatalf("logout failed: %v", err)
	}
}

func TestRefreshFailureUsesStableSessionExpiredCode(t *testing.T) {
	repo := &fakeRepository{user: testUser(t), rotateErr: ErrSessionInvalid}
	service := NewService(repo, NewTokenManager("test-access-secret-at-least-32-bytes", time.Minute), time.Hour)
	_, _, err := service.Refresh(context.Background(), "old-refresh", "", "")
	var appErr *apperror.Error
	if !errors.As(err, &appErr) || appErr.Code != "SESSION_EXPIRED" {
		t.Fatalf("unexpected error: %v", err)
	}
}
