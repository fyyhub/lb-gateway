package store

import (
	"context"
	"fmt"
)

func (s *Store) Migrate(ctx context.Context) error {
	if err := ping(ctx, s.db); err != nil {
		return err
	}

	statements := []string{
		`CREATE TABLE IF NOT EXISTS upstream_groups (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			strategy TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS upstream_targets (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			url TEXT NOT NULL,
			weight INTEGER NOT NULL DEFAULT 1,
			enabled INTEGER NOT NULL DEFAULT 1,
			health_status TEXT NOT NULL DEFAULT 'unknown',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(group_id) REFERENCES upstream_groups(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS routes (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			priority INTEGER NOT NULL DEFAULT 0,
			type TEXT NOT NULL,
			match_host TEXT NOT NULL DEFAULT '',
			match_path TEXT NOT NULL,
			match_methods_json TEXT NOT NULL DEFAULT '[]',
			upstream_group_id TEXT,
			request_rewrite_json TEXT NOT NULL DEFAULT '[]',
			response_mapping_json TEXT NOT NULL DEFAULT '[]',
			redirect_config_json TEXT,
			max_response_bytes INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(upstream_group_id) REFERENCES upstream_groups(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS request_logs (
			id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			route_id TEXT,
			upstream_url TEXT,
			status_code INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			client_ip TEXT NOT NULL,
			error TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_request_logs_created_at
			ON request_logs(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id TEXT PRIMARY KEY,
			admin_user_id TEXT,
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT,
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at
			ON audit_logs(created_at DESC)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("run migration statement: %w", err)
		}
	}

	return nil
}
