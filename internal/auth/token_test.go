package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccessTokenRoundTripAndTamperRejection(t *testing.T) {
	now := time.Now().UTC()
	user := User{ID: uuid.New(), StoreID: uuid.New(), AuthVersion: 3}
	manager := NewTokenManager("test-access-secret-at-least-32-bytes", 15*time.Minute)
	raw, expires, err := manager.Access(user, now)
	if err != nil {
		t.Fatal(err)
	}
	if expires != 900 {
		t.Fatalf("expires = %d", expires)
	}
	claims, err := manager.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != user.ID.String() || claims.StoreID != user.StoreID.String() || claims.AuthVersion != 3 {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if _, err := manager.Parse(raw + "tampered"); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestRefreshTokensAreRandomAndStoredAsHashes(t *testing.T) {
	one, hashOne, err := newRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	two, hashTwo, err := newRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if one == two || string(hashOne) == string(hashTwo) {
		t.Fatal("refresh tokens must be unique")
	}
	if one == string(hashOne) || string(hashRefreshToken(one)) != string(hashOne) {
		t.Fatal("refresh hash mismatch")
	}
}
