package store

import (
	"context"
	"database/sql"
	"fmt"

	"light-api-gateway/internal/config"
)

func (s *Store) CountRoutes(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM routes`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count routes: %w", err)
	}
	return count, nil
}

func (s *Store) ListRoutes(ctx context.Context) ([]config.RouteConfig, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, enabled, priority, type, match_host, match_path, match_methods_json,
			upstream_group_id, request_rewrite_json, response_mapping_json, redirect_config_json, max_response_bytes
		FROM routes
		ORDER BY priority DESC, created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	defer rows.Close()

	var routes []config.RouteConfig
	for rows.Next() {
		route, err := scanRoute(rows)
		if err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate routes: %w", err)
	}

	return routes, nil
}

func (s *Store) ListEnabledRuntimeRoutes(ctx context.Context) ([]config.RouteConfig, error) {
	routes, err := s.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}

	var runtimeRoutes []config.RouteConfig
	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		if route.Type == "proxy" && route.UpstreamGroupID != "" {
			group, err := s.GetUpstreamGroup(ctx, route.UpstreamGroupID)
			if err != nil {
				return nil, fmt.Errorf("load route %q upstream group: %w", route.Name, err)
			}
			route.UpstreamGroup = group
		}
		runtimeRoutes = append(runtimeRoutes, route)
	}

	return runtimeRoutes, nil
}

func (s *Store) GetRoute(ctx context.Context, id string) (config.RouteConfig, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, enabled, priority, type, match_host, match_path, match_methods_json,
			upstream_group_id, request_rewrite_json, response_mapping_json, redirect_config_json, max_response_bytes
		FROM routes
		WHERE id = ?
	`, id)
	return scanRoute(row)
}

func (s *Store) CreateRoute(ctx context.Context, route config.RouteConfig) (config.RouteConfig, error) {
	if route.ID == "" {
		route.ID = newID("rte")
	}
	if route.Name == "" {
		route.Name = route.ID
	}

	methodsJSON, err := marshalJSON(route.Match.Methods)
	if err != nil {
		return config.RouteConfig{}, err
	}
	requestRewriteJSON, err := marshalJSON(route.RequestRewrite)
	if err != nil {
		return config.RouteConfig{}, err
	}
	responseMappingJSON, err := marshalJSON(route.ResponseMapping)
	if err != nil {
		return config.RouteConfig{}, err
	}
	redirectJSON, err := marshalJSON(route.Redirect)
	if err != nil {
		return config.RouteConfig{}, err
	}

	now := nowText()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO routes (
			id, name, enabled, priority, type, match_host, match_path, match_methods_json,
			upstream_group_id, request_rewrite_json, response_mapping_json, redirect_config_json,
			max_response_bytes, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, route.ID, route.Name, boolToInt(route.Enabled), route.Priority, route.Type, route.Match.Host, route.Match.Path,
		methodsJSON, nullableString(route.UpstreamGroupID), requestRewriteJSON, responseMappingJSON,
		nullableJSONString(redirectJSON), route.MaxResponseBytes, now, now)
	if err != nil {
		return config.RouteConfig{}, fmt.Errorf("create route: %w", err)
	}

	return s.GetRoute(ctx, route.ID)
}

func (s *Store) UpdateRoute(ctx context.Context, id string, route config.RouteConfig) (config.RouteConfig, error) {
	if route.Name == "" {
		route.Name = id
	}

	methodsJSON, err := marshalJSON(route.Match.Methods)
	if err != nil {
		return config.RouteConfig{}, err
	}
	requestRewriteJSON, err := marshalJSON(route.RequestRewrite)
	if err != nil {
		return config.RouteConfig{}, err
	}
	responseMappingJSON, err := marshalJSON(route.ResponseMapping)
	if err != nil {
		return config.RouteConfig{}, err
	}
	redirectJSON, err := marshalJSON(route.Redirect)
	if err != nil {
		return config.RouteConfig{}, err
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE routes
		SET name = ?, enabled = ?, priority = ?, type = ?, match_host = ?, match_path = ?,
			match_methods_json = ?, upstream_group_id = ?, request_rewrite_json = ?,
			response_mapping_json = ?, redirect_config_json = ?, max_response_bytes = ?, updated_at = ?
		WHERE id = ?
	`, route.Name, boolToInt(route.Enabled), route.Priority, route.Type, route.Match.Host, route.Match.Path,
		methodsJSON, nullableString(route.UpstreamGroupID), requestRewriteJSON, responseMappingJSON,
		nullableJSONString(redirectJSON), route.MaxResponseBytes, nowText(), id)
	if err != nil {
		return config.RouteConfig{}, fmt.Errorf("update route: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return config.RouteConfig{}, sql.ErrNoRows
	}

	return s.GetRoute(ctx, id)
}

func (s *Store) DeleteRoute(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM routes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete route: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetRouteEnabled(ctx context.Context, id string, enabled bool) (config.RouteConfig, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE routes
		SET enabled = ?, updated_at = ?
		WHERE id = ?
	`, boolToInt(enabled), nowText(), id)
	if err != nil {
		return config.RouteConfig{}, fmt.Errorf("set route enabled: %w", err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		return config.RouteConfig{}, sql.ErrNoRows
	}

	return s.GetRoute(ctx, id)
}

type routeScanner interface {
	Scan(dest ...any) error
}

func scanRoute(scanner routeScanner) (config.RouteConfig, error) {
	var route config.RouteConfig
	var enabled int
	var methodsJSON string
	var upstreamGroupID sql.NullString
	var requestRewriteJSON string
	var responseMappingJSON string
	var redirectJSON sql.NullString

	err := scanner.Scan(
		&route.ID,
		&route.Name,
		&enabled,
		&route.Priority,
		&route.Type,
		&route.Match.Host,
		&route.Match.Path,
		&methodsJSON,
		&upstreamGroupID,
		&requestRewriteJSON,
		&responseMappingJSON,
		&redirectJSON,
		&route.MaxResponseBytes,
	)
	if err != nil {
		return config.RouteConfig{}, err
	}

	route.Enabled = intToBool(enabled)
	if upstreamGroupID.Valid {
		route.UpstreamGroupID = upstreamGroupID.String
	}
	if err := unmarshalJSON(methodsJSON, &route.Match.Methods); err != nil {
		return config.RouteConfig{}, err
	}
	if err := unmarshalJSON(requestRewriteJSON, &route.RequestRewrite); err != nil {
		return config.RouteConfig{}, err
	}
	if err := unmarshalJSON(responseMappingJSON, &route.ResponseMapping); err != nil {
		return config.RouteConfig{}, err
	}
	if redirectJSON.Valid && redirectJSON.String != "null" {
		var redirect config.RedirectConfig
		if err := unmarshalJSON(redirectJSON.String, &redirect); err != nil {
			return config.RouteConfig{}, err
		}
		route.Redirect = &redirect
	}

	return route, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableJSONString(value string) any {
	if value == "" || value == "null" {
		return nil
	}
	return value
}
