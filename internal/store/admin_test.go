package store

import (
	"context"
	"errors"
	"testing"

	"light-api-gateway/internal/auth"
)

func TestAdminCredentialsUpdate(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	user, created, err := st.EnsureAdminUser(ctx, "admin", "admin123456")
	if err != nil || !created {
		t.Fatalf("EnsureAdminUser returned err=%v created=%v", err, created)
	}

	byID, err := st.GetAdminUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetAdminUserByID returned error: %v", err)
	}
	if byID.Username != "admin" {
		t.Fatalf("got username %q, want admin", byID.Username)
	}

	newHash, err := auth.HashPassword("new-strong-password")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	updated, err := st.UpdateAdminCredentials(ctx, user.ID, "operator", newHash)
	if err != nil {
		t.Fatalf("UpdateAdminCredentials returned error: %v", err)
	}
	if updated.Username != "operator" {
		t.Fatalf("got username %q, want operator", updated.Username)
	}

	reloaded, err := st.GetAdminUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetAdminUserByID returned error: %v", err)
	}
	if !auth.VerifyPassword("new-strong-password", reloaded.PasswordHash) {
		t.Fatal("expected the new password to verify after update")
	}

	// A second account occupying a username must block renaming onto it.
	if _, _, err := st.EnsureAdminUser(ctx, "taken", "anotherpw123"); err != nil {
		t.Fatalf("EnsureAdminUser(second) returned error: %v", err)
	}
	if _, err := st.UpdateAdminCredentials(ctx, user.ID, "taken", reloaded.PasswordHash); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("got err %v, want ErrUsernameTaken", err)
	}
}
