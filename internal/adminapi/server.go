package adminapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"light-api-gateway/internal/auth"
	"light-api-gateway/internal/config"
	"light-api-gateway/internal/mapping"
	"light-api-gateway/internal/store"
)

const maxDebugResponseBytes = 1 << 20

type Server struct {
	store  *store.Store
	tokens *auth.TokenManager
	logger *slog.Logger
}

func New(st *store.Store, tokens *auth.TokenManager, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		store:  st,
		tokens: tokens,
		logger: logger,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setCommonHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.URL.Path == "/admin/api/health" {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	if r.URL.Path == "/admin/api/auth/login" {
		s.handleLogin(w, r)
		return
	}

	claims, ok := s.authenticate(w, r)
	if !ok {
		return
	}

	switch {
	case r.URL.Path == "/admin/api/auth/me":
		s.handleMe(w, r, claims)
	case r.URL.Path == "/admin/api/routes":
		s.handleRoutes(w, r, claims)
	case strings.HasPrefix(r.URL.Path, "/admin/api/routes/"):
		s.handleRouteByID(w, r, claims)
	case r.URL.Path == "/admin/api/upstream-groups":
		s.handleUpstreamGroups(w, r, claims)
	case strings.HasPrefix(r.URL.Path, "/admin/api/upstream-groups/"):
		s.handleUpstreamGroupByID(w, r, claims)
	case strings.HasPrefix(r.URL.Path, "/admin/api/upstream-targets/"):
		s.handleUpstreamTargetByID(w, r, claims)
	case r.URL.Path == "/admin/api/request-logs":
		s.handleRequestLogs(w, r)
	case r.URL.Path == "/admin/api/audit-logs":
		s.handleAuditLogs(w, r)
	case r.URL.Path == "/admin/api/debug/request":
		s.handleDebugRequest(w, r)
	case r.URL.Path == "/admin/api/debug/mapping":
		s.handleMappingPreview(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func setCommonHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (auth.Claims, bool) {
	header := r.Header.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" || token == header {
		writeError(w, http.StatusUnauthorized, "missing bearer token")
		return auth.Claims{}, false
	}

	claims, err := s.tokens.Verify(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid bearer token")
		return auth.Claims{}, false
	}

	return claims, true
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	user, err := s.store.GetAdminUserByUsername(r.Context(), req.Username)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err != nil {
		s.logger.Error("load admin user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "load admin user failed")
		return
	}
	if !user.Enabled || !auth.VerifyPassword(req.Password, user.PasswordHash) {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, err := s.tokens.Issue(user.ID, user.Username, user.Role)
	if err != nil {
		s.logger.Error("issue token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "issue token failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, claims)
}

func (s *Server) handleRoutes(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	switch r.Method {
	case http.MethodGet:
		routes, err := s.store.ListRoutes(r.Context())
		if err != nil {
			s.logger.Error("list routes failed", "error", err)
			writeError(w, http.StatusInternalServerError, "list routes failed")
			return
		}
		writeJSON(w, http.StatusOK, routes)
	case http.MethodPost:
		var route config.RouteConfig
		if !readJSON(w, r, &route) {
			return
		}
		created, err := s.store.CreateRoute(r.Context(), route)
		if err != nil {
			s.logger.Error("create route failed", "error", err)
			writeError(w, http.StatusBadRequest, "create route failed")
			return
		}
		s.recordAudit(r, claims, "create", "route", created.ID, map[string]any{
			"name":    created.Name,
			"type":    created.Type,
			"path":    created.Match.Path,
			"enabled": created.Enabled,
		})
		writeJSON(w, http.StatusCreated, created)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleRouteByID(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	parts := pathParts(r.URL.Path, "/admin/api/routes/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "route not found")
		return
	}
	id := parts[0]

	if len(parts) == 2 && parts[1] == "enabled" {
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		route, err := s.store.SetRouteEnabled(r.Context(), id, req.Enabled)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "resource not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database operation failed")
			return
		}
		s.recordAudit(r, claims, "set_enabled", "route", route.ID, map[string]any{
			"name":    route.Name,
			"enabled": route.Enabled,
		})
		writeJSON(w, http.StatusOK, route)
		return
	}

	switch r.Method {
	case http.MethodGet:
		route, err := s.store.GetRoute(r.Context(), id)
		writeStoreResult(w, route, err, http.StatusOK)
	case http.MethodPut:
		var route config.RouteConfig
		if !readJSON(w, r, &route) {
			return
		}
		updated, err := s.store.UpdateRoute(r.Context(), id, route)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "resource not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database operation failed")
			return
		}
		s.recordAudit(r, claims, "update", "route", updated.ID, map[string]any{
			"name":    updated.Name,
			"type":    updated.Type,
			"path":    updated.Match.Path,
			"enabled": updated.Enabled,
		})
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		err := s.store.DeleteRoute(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "route not found")
			return
		}
		if err != nil {
			s.logger.Error("delete route failed", "error", err)
			writeError(w, http.StatusInternalServerError, "delete route failed")
			return
		}
		s.recordAudit(r, claims, "delete", "route", id, nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleUpstreamGroups(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	switch r.Method {
	case http.MethodGet:
		groups, err := s.store.ListUpstreamGroups(r.Context())
		if err != nil {
			s.logger.Error("list upstream groups failed", "error", err)
			writeError(w, http.StatusInternalServerError, "list upstream groups failed")
			return
		}
		writeJSON(w, http.StatusOK, groups)
	case http.MethodPost:
		var group config.UpstreamGroup
		if !readJSON(w, r, &group) {
			return
		}
		created, err := s.store.CreateUpstreamGroup(r.Context(), group)
		if err != nil {
			s.logger.Error("create upstream group failed", "error", err)
			writeError(w, http.StatusBadRequest, "create upstream group failed")
			return
		}
		s.recordAudit(r, claims, "create", "upstream_group", created.ID, map[string]any{
			"name":     created.Name,
			"strategy": created.Strategy,
		})
		writeJSON(w, http.StatusCreated, created)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleUpstreamGroupByID(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	parts := pathParts(r.URL.Path, "/admin/api/upstream-groups/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "upstream group not found")
		return
	}
	id := parts[0]

	if len(parts) == 2 && parts[1] == "targets" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var target config.TargetConfig
		if !readJSON(w, r, &target) {
			return
		}
		target.GroupID = id
		created, err := s.store.CreateUpstreamTarget(r.Context(), target)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "resource not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database operation failed")
			return
		}
		s.recordAudit(r, claims, "create", "upstream_target", created.ID, map[string]any{
			"groupId": created.GroupID,
			"url":     created.URL,
			"enabled": created.Enabled,
			"weight":  created.Weight,
		})
		writeJSON(w, http.StatusCreated, created)
		return
	}

	switch r.Method {
	case http.MethodGet:
		group, err := s.store.GetUpstreamGroup(r.Context(), id)
		writeStoreResult(w, group, err, http.StatusOK)
	case http.MethodPut:
		var group config.UpstreamGroup
		if !readJSON(w, r, &group) {
			return
		}
		updated, err := s.store.UpdateUpstreamGroup(r.Context(), id, group)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "resource not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database operation failed")
			return
		}
		s.recordAudit(r, claims, "update", "upstream_group", updated.ID, map[string]any{
			"name":     updated.Name,
			"strategy": updated.Strategy,
		})
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		err := s.store.DeleteUpstreamGroup(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "upstream group not found")
			return
		}
		if err != nil {
			s.logger.Error("delete upstream group failed", "error", err)
			writeError(w, http.StatusInternalServerError, "delete upstream group failed")
			return
		}
		s.recordAudit(r, claims, "delete", "upstream_group", id, nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleUpstreamTargetByID(w http.ResponseWriter, r *http.Request, claims auth.Claims) {
	parts := pathParts(r.URL.Path, "/admin/api/upstream-targets/")
	if len(parts) != 1 || parts[0] == "" {
		writeError(w, http.StatusNotFound, "upstream target not found")
		return
	}
	id := parts[0]

	switch r.Method {
	case http.MethodPut:
		var target config.TargetConfig
		if !readJSON(w, r, &target) {
			return
		}
		updated, err := s.store.UpdateUpstreamTarget(r.Context(), id, target)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "resource not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "database operation failed")
			return
		}
		s.recordAudit(r, claims, "update", "upstream_target", updated.ID, map[string]any{
			"groupId": updated.GroupID,
			"url":     updated.URL,
			"enabled": updated.Enabled,
			"weight":  updated.Weight,
		})
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		err := s.store.DeleteUpstreamTarget(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "upstream target not found")
			return
		}
		if err != nil {
			s.logger.Error("delete upstream target failed", "error", err)
			writeError(w, http.StatusInternalServerError, "delete upstream target failed")
			return
		}
		s.recordAudit(r, claims, "delete", "upstream_target", id, nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (s *Server) handleMappingPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Source any                  `json:"source"`
		Rules  []config.MappingRule `json:"rules"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	result, err := mapping.Apply(req.Source, req.Rules)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"result": result,
	})
}

func (s *Server) handleRequestLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := 100
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}

	logs, err := s.store.ListRequestLogs(r.Context(), limit)
	if err != nil {
		s.logger.Error("list request logs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list request logs failed")
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := 100
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}

	logs, err := s.store.ListAuditLogs(r.Context(), limit)
	if err != nil {
		s.logger.Error("list audit logs failed", "error", err)
		writeError(w, http.StatusInternalServerError, "list audit logs failed")
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

func (s *Server) handleDebugRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		URL       string            `json:"url"`
		Method    string            `json:"method"`
		Headers   map[string]string `json:"headers"`
		Body      string            `json:"body"`
		TimeoutMS int               `json:"timeoutMs"`
	}
	if !readJSON(w, r, &req) {
		return
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !isAllowedDebugMethod(method) {
		writeError(w, http.StatusBadRequest, "unsupported debug request method")
		return
	}

	targetURL, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || targetURL.Host == "" {
		writeError(w, http.StatusBadRequest, "invalid debug request url")
		return
	}
	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		writeError(w, http.StatusBadRequest, "debug request url must use http or https")
		return
	}
	if !isLoopbackHost(targetURL.Hostname()) {
		writeError(w, http.StatusBadRequest, "debug request url must target localhost or loopback")
		return
	}

	timeout := debugTimeout(req.TimeoutMS)
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	outbound, err := http.NewRequestWithContext(ctx, method, targetURL.String(), strings.NewReader(req.Body))
	if err != nil {
		writeError(w, http.StatusBadRequest, "create debug request failed")
		return
	}
	for key, value := range req.Headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		outbound.Header.Set(key, value)
	}

	start := time.Now()
	resp, err := (&http.Client{Timeout: timeout}).Do(outbound)
	duration := time.Since(start)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDebugResponseBytes+1))
	if err != nil {
		writeError(w, http.StatusBadGateway, "read debug response failed")
		return
	}
	truncated := len(body) > maxDebugResponseBytes
	if truncated {
		body = body[:maxDebugResponseBytes]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"statusCode": resp.StatusCode,
		"durationMs": duration.Milliseconds(),
		"headers":    resp.Header,
		"body":       string(body),
		"truncated":  truncated,
	})
}

func isAllowedDebugMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
		return true
	default:
		return false
	}
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func debugTimeout(timeoutMS int) time.Duration {
	if timeoutMS <= 0 {
		return 5 * time.Second
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond
	if timeout < time.Second {
		return time.Second
	}
	if timeout > 10*time.Second {
		return 10 * time.Second
	}
	return timeout
}

func (s *Server) recordAudit(r *http.Request, claims auth.Claims, action string, resourceType string, resourceID string, detail map[string]any) {
	if detail == nil {
		detail = map[string]any{}
	}
	detail["username"] = claims.Username
	detail["role"] = claims.Role
	if _, err := s.store.CreateAuditLog(r.Context(), store.AuditLog{
		AdminUserID:  claims.Subject,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Detail:       detail,
	}); err != nil {
		s.logger.Warn("create audit log failed", "error", err, "action", action, "resource_type", resourceType, "resource_id", resourceID)
	}
}

func writeStoreResult(w http.ResponseWriter, value any, err error, statusCode int) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "resource not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "database operation failed")
		return
	}
	writeJSON(w, statusCode, value)
}

func pathParts(path string, prefix string) []string {
	trimmed := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.WriteHeader(statusCode)
	if statusCode == http.StatusNoContent {
		return
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Default().Error("write json response failed", "error", err)
	}
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]any{
		"error": message,
	})
}
