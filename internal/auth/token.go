package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	StoreID     string `json:"storeId"`
	AuthVersion int64  `json:"authVersion"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret  []byte
	expires time.Duration
}

func NewTokenManager(secret string, expires time.Duration) TokenManager {
	return TokenManager{secret: []byte(secret), expires: expires}
}

func (m TokenManager) Access(user User, now time.Time) (string, int64, error) {
	claims := Claims{StoreID: user.StoreID.String(), AuthVersion: user.AuthVersion, RegisteredClaims: jwt.RegisteredClaims{
		Subject: user.ID.String(), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(m.expires)), ID: uuid.NewString(), Issuer: "cloud-ledger-backend",
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	return token, int64(m.expires.Seconds()), err
}

func (m TokenManager) Parse(raw string) (Claims, error) {
	claims := Claims{}
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.secret, nil
	}, jwt.WithIssuer("cloud-ledger-backend"), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return Claims{}, fmt.Errorf("invalid access token")
	}
	return claims, nil
}

func newRefreshToken() (raw string, hash []byte, err error) {
	value := make([]byte, 32)
	if _, err = rand.Read(value); err != nil {
		return "", nil, err
	}
	raw = base64.RawURLEncoding.EncodeToString(value)
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}

func hashRefreshToken(raw string) []byte { sum := sha256.Sum256([]byte(raw)); return sum[:] }
