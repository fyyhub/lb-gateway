package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"light-api-gateway/internal/config"
)

func (s *Store) ListUpstreamGroups(ctx context.Context) ([]config.UpstreamGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, strategy
		FROM upstream_groups
		ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list upstream groups: %w", err)
	}
	defer rows.Close()

	var groups []config.UpstreamGroup
	for rows.Next() {
		var group config.UpstreamGroup
		if err := rows.Scan(&group.ID, &group.Name, &group.Strategy); err != nil {
			return nil, fmt.Errorf("scan upstream group: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream groups: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close upstream group rows: %w", err)
	}

	for i := range groups {
		targets, err := s.ListUpstreamTargets(ctx, groups[i].ID)
		if err != nil {
			return nil, err
		}
		groups[i].Targets = targets
	}

	return groups, nil
}

func (s *Store) GetUpstreamGroup(ctx context.Context, id string) (config.UpstreamGroup, error) {
	var group config.UpstreamGroup
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, strategy
		FROM upstream_groups
		WHERE id = ?
	`, id).Scan(&group.ID, &group.Name, &group.Strategy)
	if err != nil {
		return config.UpstreamGroup{}, err
	}

	targets, err := s.ListUpstreamTargets(ctx, group.ID)
	if err != nil {
		return config.UpstreamGroup{}, err
	}
	group.Targets = targets
	return group, nil
}

func (s *Store) CreateUpstreamGroup(ctx context.Context, group config.UpstreamGroup) (config.UpstreamGroup, error) {
	if group.ID == "" {
		group.ID = newID("upg")
	}
	if group.Name == "" {
		group.Name = group.ID
	}
	if group.Strategy == "" {
		group.Strategy = "round-robin"
	}

	now := nowText()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO upstream_groups (id, name, strategy, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, group.ID, group.Name, group.Strategy, now, now)
	if err != nil {
		return config.UpstreamGroup{}, fmt.Errorf("create upstream group: %w", err)
	}

	for _, target := range group.Targets {
		target.GroupID = group.ID
		if _, err := s.CreateUpstreamTarget(ctx, target); err != nil {
			return config.UpstreamGroup{}, err
		}
	}

	return s.GetUpstreamGroup(ctx, group.ID)
}

func (s *Store) UpdateUpstreamGroup(ctx context.Context, id string, group config.UpstreamGroup) (config.UpstreamGroup, error) {
	if group.Name == "" {
		group.Name = id
	}
	if group.Strategy == "" {
		group.Strategy = "round-robin"
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE upstream_groups
		SET name = ?, strategy = ?, updated_at = ?
		WHERE id = ?
	`, group.Name, group.Strategy, nowText(), id)
	if err != nil {
		return config.UpstreamGroup{}, fmt.Errorf("update upstream group: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return config.UpstreamGroup{}, sql.ErrNoRows
	}

	return s.GetUpstreamGroup(ctx, id)
}

func (s *Store) DeleteUpstreamGroup(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM upstream_groups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete upstream group: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListUpstreamTargets(ctx context.Context, groupID string) ([]config.TargetConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, group_id, url, weight, enabled, health_status, consecutive_failures
		FROM upstream_targets
		WHERE group_id = ?
		ORDER BY created_at ASC
	`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list upstream targets: %w", err)
	}
	defer rows.Close()

	var targets []config.TargetConfig
	for rows.Next() {
		var target config.TargetConfig
		var enabled int
		if err := rows.Scan(&target.ID, &target.GroupID, &target.URL, &target.Weight, &enabled, &target.HealthStatus, &target.ConsecutiveFailures); err != nil {
			return nil, fmt.Errorf("scan upstream target: %w", err)
		}
		target.Enabled = intToBool(enabled)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream targets: %w", err)
	}

	return targets, nil
}

func (s *Store) ListAllUpstreamTargets(ctx context.Context) ([]config.TargetConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, group_id, url, weight, enabled, health_status, consecutive_failures
		FROM upstream_targets
		ORDER BY group_id ASC, created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list all upstream targets: %w", err)
	}
	defer rows.Close()

	var targets []config.TargetConfig
	for rows.Next() {
		var target config.TargetConfig
		var enabled int
		if err := rows.Scan(&target.ID, &target.GroupID, &target.URL, &target.Weight, &enabled, &target.HealthStatus, &target.ConsecutiveFailures); err != nil {
			return nil, fmt.Errorf("scan upstream target: %w", err)
		}
		target.Enabled = intToBool(enabled)
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate upstream targets: %w", err)
	}

	return targets, nil
}

func (s *Store) CreateUpstreamTarget(ctx context.Context, target config.TargetConfig) (config.TargetConfig, error) {
	if target.ID == "" {
		target.ID = newID("upt")
	}
	if target.Weight <= 0 {
		target.Weight = 1
	}
	target.HealthStatus = normalizeHealthStatus(target.HealthStatus)
	target.ConsecutiveFailures = normalizeFailureCount(target.ConsecutiveFailures)

	now := nowText()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO upstream_targets (
			id, group_id, url, weight, enabled, health_status, consecutive_failures, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, target.ID, target.GroupID, target.URL, target.Weight, boolToInt(target.Enabled), target.HealthStatus, target.ConsecutiveFailures, now, now)
	if err != nil {
		return config.TargetConfig{}, fmt.Errorf("create upstream target: %w", err)
	}

	return s.GetUpstreamTarget(ctx, target.ID)
}

func (s *Store) GetUpstreamTarget(ctx context.Context, id string) (config.TargetConfig, error) {
	var target config.TargetConfig
	var enabled int
	err := s.db.QueryRowContext(ctx, `
		SELECT id, group_id, url, weight, enabled, health_status, consecutive_failures
		FROM upstream_targets
		WHERE id = ?
	`, id).Scan(&target.ID, &target.GroupID, &target.URL, &target.Weight, &enabled, &target.HealthStatus, &target.ConsecutiveFailures)
	if err != nil {
		return config.TargetConfig{}, err
	}
	target.Enabled = intToBool(enabled)
	return target, nil
}

func (s *Store) UpdateUpstreamTarget(ctx context.Context, id string, target config.TargetConfig) (config.TargetConfig, error) {
	if target.Weight <= 0 {
		target.Weight = 1
	}
	target.HealthStatus = normalizeHealthStatus(target.HealthStatus)
	target.ConsecutiveFailures = normalizeFailureCount(target.ConsecutiveFailures)

	result, err := s.db.ExecContext(ctx, `
		UPDATE upstream_targets
		SET url = ?, weight = ?, enabled = ?, health_status = ?, consecutive_failures = ?, updated_at = ?
		WHERE id = ?
	`, target.URL, target.Weight, boolToInt(target.Enabled), target.HealthStatus, target.ConsecutiveFailures, nowText(), id)
	if err != nil {
		return config.TargetConfig{}, fmt.Errorf("update upstream target: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return config.TargetConfig{}, sql.ErrNoRows
	}

	return s.GetUpstreamTarget(ctx, id)
}

func (s *Store) SetUpstreamTargetHealthStatus(ctx context.Context, id string, status string) (config.TargetConfig, error) {
	status = normalizeHealthStatus(status)
	result, err := s.db.ExecContext(ctx, `
		UPDATE upstream_targets
		SET health_status = ?, updated_at = ?
		WHERE id = ?
	`, status, nowText(), id)
	if err != nil {
		return config.TargetConfig{}, fmt.Errorf("set upstream target health status: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return config.TargetConfig{}, sql.ErrNoRows
	}

	return s.GetUpstreamTarget(ctx, id)
}

func (s *Store) RecordUpstreamTargetHealthCheck(ctx context.Context, id string, status string, failed bool, autoDisableThreshold int) (config.TargetConfig, error) {
	status = normalizeHealthStatus(status)
	failedInt := boolToInt(failed)
	now := nowText()

	result, err := s.db.ExecContext(ctx, `
		UPDATE upstream_targets
		SET health_status = ?,
			consecutive_failures = CASE
				WHEN ? = 1 THEN consecutive_failures + 1
				ELSE 0
			END,
			enabled = CASE
				WHEN ? > 0 AND ? = 1 AND consecutive_failures + 1 >= ? THEN 0
				ELSE enabled
			END,
			updated_at = ?
		WHERE id = ?
	`, status, failedInt, autoDisableThreshold, failedInt, autoDisableThreshold, now, id)
	if err != nil {
		return config.TargetConfig{}, fmt.Errorf("record upstream target health check: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return config.TargetConfig{}, sql.ErrNoRows
	}

	return s.GetUpstreamTarget(ctx, id)
}

func (s *Store) DeleteUpstreamTarget(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM upstream_targets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete upstream target: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func normalizeHealthStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "healthy", "unhealthy":
		return status
	default:
		return "unknown"
	}
}

func normalizeFailureCount(count int) int {
	if count < 0 {
		return 0
	}
	return count
}
