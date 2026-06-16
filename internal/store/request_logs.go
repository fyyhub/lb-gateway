package store

import (
	"context"
	"database/sql"
	"fmt"
)

type RequestLog struct {
	ID          string `json:"id"`
	RequestID   string `json:"requestId"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	RouteID     string `json:"routeId,omitempty"`
	UpstreamURL string `json:"upstreamUrl,omitempty"`
	StatusCode  int    `json:"statusCode"`
	DurationMS  int64  `json:"durationMs"`
	ClientIP    string `json:"clientIp"`
	Error       string `json:"error,omitempty"`
	CreatedAt   string `json:"createdAt"`
}

func (s *Store) CreateRequestLog(ctx context.Context, log RequestLog) (RequestLog, error) {
	if log.ID == "" {
		log.ID = newID("rlg")
	}
	if log.RequestID == "" {
		log.RequestID = newID("req")
	}
	if log.CreatedAt == "" {
		log.CreatedAt = nowText()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO request_logs (
			id, request_id, method, path, route_id, upstream_url, status_code,
			duration_ms, client_ip, error, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, log.ID, log.RequestID, log.Method, log.Path, nullableString(log.RouteID),
		nullableString(log.UpstreamURL), log.StatusCode, log.DurationMS, log.ClientIP,
		nullableString(log.Error), log.CreatedAt)
	if err != nil {
		return RequestLog{}, fmt.Errorf("create request log: %w", err)
	}

	return log, nil
}

func (s *Store) ListRequestLogs(ctx context.Context, limit int) ([]RequestLog, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, request_id, method, path, route_id, upstream_url, status_code,
			duration_ms, client_ip, error, created_at
		FROM request_logs
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list request logs: %w", err)
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		log, err := scanRequestLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate request logs: %w", err)
	}

	return logs, nil
}

type requestLogScanner interface {
	Scan(dest ...any) error
}

func scanRequestLog(scanner requestLogScanner) (RequestLog, error) {
	var log RequestLog
	var routeID sql.NullString
	var upstreamURL sql.NullString
	var errText sql.NullString
	err := scanner.Scan(
		&log.ID,
		&log.RequestID,
		&log.Method,
		&log.Path,
		&routeID,
		&upstreamURL,
		&log.StatusCode,
		&log.DurationMS,
		&log.ClientIP,
		&errText,
		&log.CreatedAt,
	)
	if err != nil {
		return RequestLog{}, fmt.Errorf("scan request log: %w", err)
	}
	if routeID.Valid {
		log.RouteID = routeID.String
	}
	if upstreamURL.Valid {
		log.UpstreamURL = upstreamURL.String
	}
	if errText.Valid {
		log.Error = errText.String
	}
	return log, nil
}
