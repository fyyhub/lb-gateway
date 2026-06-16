package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"light-api-gateway/internal/auth"
)

type AdminUser struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
}

// ErrUsernameTaken is returned when updating an admin account to a username that
// already belongs to a different user.
var ErrUsernameTaken = errors.New("username already taken")

func (s *Store) CountAdminUsers(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count admin users: %w", err)
	}
	return count, nil
}

func (s *Store) EnsureAdminUser(ctx context.Context, username string, password string) (AdminUser, bool, error) {
	user, err := s.GetAdminUserByUsername(ctx, username)
	if err == nil {
		return user, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AdminUser{}, false, err
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return AdminUser{}, false, err
	}

	now := nowText()
	user = AdminUser{
		ID:           newID("usr"),
		Username:     username,
		PasswordHash: passwordHash,
		Role:         "admin",
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO admin_users (id, username, password_hash, role, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, user.ID, user.Username, user.PasswordHash, user.Role, boolToInt(user.Enabled), user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return AdminUser{}, false, fmt.Errorf("create admin user: %w", err)
	}

	return user, true, nil
}

func (s *Store) GetAdminUserByUsername(ctx context.Context, username string) (AdminUser, error) {
	var user AdminUser
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, enabled, created_at, updated_at
		FROM admin_users
		WHERE username = ?
	`, username).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &enabled, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return AdminUser{}, err
	}
	user.Enabled = intToBool(enabled)
	return user, nil
}

func (s *Store) GetAdminUserByID(ctx context.Context, id string) (AdminUser, error) {
	var user AdminUser
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, enabled, created_at, updated_at
		FROM admin_users
		WHERE id = ?
	`, id).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &enabled, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return AdminUser{}, err
	}
	user.Enabled = intToBool(enabled)
	return user, nil
}

// UpdateAdminCredentials updates the username and password hash for an admin
// user. Pass the existing hash to leave the password unchanged. It returns
// ErrUsernameTaken if the username already belongs to a different account.
func (s *Store) UpdateAdminCredentials(ctx context.Context, id string, username string, passwordHash string) (AdminUser, error) {
	existing, err := s.GetAdminUserByUsername(ctx, username)
	if err == nil && existing.ID != id {
		return AdminUser{}, ErrUsernameTaken
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AdminUser{}, err
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE admin_users
		SET username = ?, password_hash = ?, updated_at = ?
		WHERE id = ?
	`, username, passwordHash, nowText(), id)
	if err != nil {
		return AdminUser{}, fmt.Errorf("update admin credentials: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return AdminUser{}, sql.ErrNoRows
	}

	return s.GetAdminUserByID(ctx, id)
}
