package auth

import (
	"testing"
	"time"
)

func TestTokenManagerIssueAndVerify(t *testing.T) {
	manager, err := NewTokenManager("test-secret", time.Hour)
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}

	token, err := manager.Issue("user-1", "admin", "admin")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	claims, err := manager.Verify(token)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if claims.Subject != "user-1" || claims.Username != "admin" || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}
