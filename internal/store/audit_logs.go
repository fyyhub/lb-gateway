package store

import (
	"context"
	"database/sql"
	"fmt"
)

type AuditLog struct {
	ID           string         `json:"id"`
	AdminUserID  string         `json:"adminUserId,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId,omitempty"`
	Detail       map[string]any `json:"detail"`
	CreatedAt    string         `json:"createdAt"`
}

func (s *Store) CreateAuditLog(ctx context.Context, log AuditLog) (AuditLog, error) {
	if log.ID == "" {
		log.ID = newID("aud")
	}
	if log.CreatedAt == "" {
		log.CreatedAt = nowText()
	}
	if log.Detail == nil {
		log.Detail = map[string]any{}
	}
	detailJSON, err := marshalJSON(log.Detail)
	if err != nil {
		return AuditLog{}, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (
			id, admin_user_id, action, resource_type, resource_id, detail_json, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, log.ID, nullableString(log.AdminUserID), log.Action, log.ResourceType,
		nullableString(log.ResourceID), detailJSON, log.CreatedAt)
	if err != nil {
		return AuditLog{}, fmt.Errorf("create audit log: %w", err)
	}

	return log, nil
}

func (s *Store) ListAuditLogs(ctx context.Context, limit int) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, admin_user_id, action, resource_type, resource_id, detail_json, created_at
		FROM audit_logs
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit logs: %w", err)
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		log, err := scanAuditLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit logs: %w", err)
	}

	return logs, nil
}

type auditLogScanner interface {
	Scan(dest ...any) error
}

func scanAuditLog(scanner auditLogScanner) (AuditLog, error) {
	var log AuditLog
	var adminUserID sql.NullString
	var resourceID sql.NullString
	var detailJSON string
	err := scanner.Scan(
		&log.ID,
		&adminUserID,
		&log.Action,
		&log.ResourceType,
		&resourceID,
		&detailJSON,
		&log.CreatedAt,
	)
	if err != nil {
		return AuditLog{}, fmt.Errorf("scan audit log: %w", err)
	}
	if adminUserID.Valid {
		log.AdminUserID = adminUserID.String
	}
	if resourceID.Valid {
		log.ResourceID = resourceID.String
	}
	if err := unmarshalJSON(detailJSON, &log.Detail); err != nil {
		return AuditLog{}, err
	}
	if log.Detail == nil {
		log.Detail = map[string]any{}
	}
	return log, nil
}
