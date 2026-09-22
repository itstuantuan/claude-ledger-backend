package config

import "testing"

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "test-access-secret-at-least-32-bytes")
	if _, err := Load(); err == nil {
		t.Fatal("expected missing DATABASE_URL to fail")
	}
}

func TestProductionRejectsWildcardCORS(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("JWT_SECRET", "test-access-secret-at-least-32-bytes")
	t.Setenv("APP_ENV", "production")
	t.Setenv("CORS_ORIGINS", "*")
	t.Setenv("COOKIE_SECURE", "true")
	if _, err := Load(); err == nil {
		t.Fatal("expected wildcard production CORS to fail")
	}
}

func TestProductionRequiresSecureCookie(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("JWT_SECRET", "test-access-secret-at-least-32-bytes")
	t.Setenv("APP_ENV", "production")
	t.Setenv("COOKIE_SECURE", "false")
	if _, err := Load(); err == nil {
		t.Fatal("expected insecure production cookie to fail")
	}
}

func TestLoadRequiresStrongJWTSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected short JWT secret to fail")
	}
}
