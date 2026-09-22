package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Database struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	PingTimeout     time.Duration
}

type HTTP struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

type Auth struct {
	AccessSecret   string
	AccessExpires  time.Duration
	RefreshExpires time.Duration
	CookieName     string
	CookieSecure   bool
}

type Config struct {
	AppEnv      string
	AppPort     string
	LogLevel    string
	CORSOrigins []string
	Database    Database
	HTTP        HTTP
	Auth        Auth
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:   value("APP_ENV", "development"),
		AppPort:  value("APP_PORT", "8080"),
		LogLevel: value("LOG_LEVEL", "info"),
		Database: Database{
			URL:             strings.TrimSpace(os.Getenv("DATABASE_URL")),
			MaxOpenConns:    intValue("DB_MAX_OPEN_CONNS", 20),
			MaxIdleConns:    intValue("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: durationValue("DB_CONN_MAX_LIFETIME", 30*time.Minute),
			PingTimeout:     durationValue("DB_PING_TIMEOUT", 3*time.Second),
		},
		HTTP: HTTP{
			ReadHeaderTimeout: durationValue("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       durationValue("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:      durationValue("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:       durationValue("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   durationValue("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Auth: Auth{
			AccessSecret:   strings.TrimSpace(os.Getenv("JWT_SECRET")),
			AccessExpires:  durationValue("JWT_ACCESS_EXPIRES", 15*time.Minute),
			RefreshExpires: durationValue("JWT_REFRESH_EXPIRES", 7*24*time.Hour),
			CookieName:     value("REFRESH_COOKIE_NAME", "cloud_ledger_refresh"),
			CookieSecure:   boolValue("COOKIE_SECURE", false),
		},
	}
	for _, origin := range strings.Split(value("CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000"), ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			cfg.CORSOrigins = append(cfg.CORSOrigins, origin)
		}
	}
	if cfg.Database.URL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.Auth.AccessSecret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must be at least 32 characters")
	}
	if cfg.AppEnv == "production" {
		if !cfg.Auth.CookieSecure {
			return Config{}, fmt.Errorf("COOKIE_SECURE must be true in production")
		}
		for _, origin := range cfg.CORSOrigins {
			if origin == "*" {
				return Config{}, fmt.Errorf("CORS_ORIGINS cannot contain * in production")
			}
		}
	}
	if _, err := strconv.Atoi(cfg.AppPort); err != nil {
		return Config{}, fmt.Errorf("APP_PORT must be numeric: %w", err)
	}
	return cfg, nil
}

func boolValue(key string, fallback bool) bool {
	v, err := strconv.ParseBool(value(key, strconv.FormatBool(fallback)))
	if err != nil {
		return fallback
	}
	return v
}

func value(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func intValue(key string, fallback int) int {
	v, err := strconv.Atoi(value(key, strconv.Itoa(fallback)))
	if err != nil || v < 1 {
		return fallback
	}
	return v
}

func durationValue(key string, fallback time.Duration) time.Duration {
	v, err := time.ParseDuration(value(key, fallback.String()))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
