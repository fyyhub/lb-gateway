package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"light-api-gateway/internal/config"
	"light-api-gateway/internal/mapping"
	"light-api-gateway/internal/rewrite"
	"light-api-gateway/internal/router"
	"light-api-gateway/internal/store"
)

const defaultMaxMappedResponseBytes = 1 << 20

type Gateway struct {
	mu         sync.RWMutex
	cfg        config.Config
	router     *router.Router
	logger     *slog.Logger
	logStore   requestLogStore
	requestSeq uint64
}

func New(cfg config.Config, logger *slog.Logger) (*Gateway, error) {
	return NewWithRequestLogStore(cfg, logger, nil)
}

type requestLogStore interface {
	CreateRequestLog(ctx context.Context, log store.RequestLog) (store.RequestLog, error)
}

func NewWithRequestLogStore(cfg config.Config, logger *slog.Logger, logStore requestLogStore) (*Gateway, error) {
	if logger == nil {
		logger = slog.Default()
	}
	rt, skipped := router.New(cfg)
	logSkippedRoutes(logger, skipped)
	return &Gateway{
		cfg:      cfg,
		router:   rt,
		logger:   logger,
		logStore: logStore,
	}, nil
}

func (g *Gateway) Reload(cfg config.Config) error {
	rt, skipped := router.New(cfg)

	g.mu.Lock()
	g.cfg = cfg
	g.router = rt
	g.mu.Unlock()

	logSkippedRoutes(g.logger, skipped)
	g.logger.Info("gateway config reloaded", "routes", rt.Len(), "skipped", len(skipped))
	return nil
}

func logSkippedRoutes(logger *slog.Logger, skipped []router.SkippedRoute) {
	for _, route := range skipped {
		logger.Warn("skipped invalid route", "route", route.Name, "reason", route.Reason)
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

	cfg, rt := g.snapshot()
	route, ok := rt.Match(r)
	if !ok {
		http.NotFound(recorder, r)
		g.logRequest(cfg.Gateway.LogRequests, r, "", "", recorder.statusCode, time.Since(start), "")
		return
	}

	var upstreamURL string
	var errText string

	switch route.Config.Type {
	case "proxy":
		upstreamURL, errText = g.handleProxy(recorder, r, route)
	case "redirect":
		upstreamURL, errText = g.handleRedirect(recorder, r, route)
	default:
		recorder.WriteHeader(http.StatusInternalServerError)
		errText = "unsupported route type"
	}

	g.logRequest(cfg.Gateway.LogRequests, r, route.Config.Name, upstreamURL, recorder.statusCode, time.Since(start), errText)
}

func (g *Gateway) snapshot() (config.Config, *router.Router) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.cfg, g.router
}

func (g *Gateway) handleProxy(w http.ResponseWriter, r *http.Request, route *router.Route) (string, string) {
	target, ok := route.UpstreamPicker.Next()
	if !ok {
		http.Error(w, "no upstream target available", http.StatusBadGateway)
		return "", "no upstream target available"
	}

	targetURL, err := url.Parse(target.URL)
	if err != nil {
		http.Error(w, "invalid upstream target", http.StatusBadGateway)
		return target.URL, err.Error()
	}

	if err := rewrite.Apply(r, route.Config.RequestRewrite); err != nil {
		http.Error(w, "request rewrite failed", http.StatusBadRequest)
		return target.URL, err.Error()
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
		req.Header.Set("X-Forwarded-Host", r.Host)
		req.Header.Set("X-Gateway-Route", route.Config.Name)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		return applyResponseMapping(resp, route.Config)
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		http.Error(rw, "upstream request failed", http.StatusBadGateway)
		g.logger.Warn("upstream request failed", "route", route.Config.Name, "target", target.URL, "error", err)
	}

	proxy.ServeHTTP(w, r)
	return target.URL, ""
}

func applyResponseMapping(resp *http.Response, route config.RouteConfig) error {
	if len(route.ResponseMapping) == 0 {
		return nil
	}
	if !isJSONResponse(resp) {
		return nil
	}

	limit := route.MaxResponseBytes
	if limit <= 0 {
		limit = defaultMaxMappedResponseBytes
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return fmt.Errorf("read upstream response body: %w", err)
	}
	resp.Body.Close()
	if int64(len(body)) > limit {
		return fmt.Errorf("upstream response body exceeds mapping limit")
	}
	if len(bytes.TrimSpace(body)) == 0 {
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		return nil
	}

	var source any
	if err := json.Unmarshal(body, &source); err != nil {
		return fmt.Errorf("decode upstream json response: %w", err)
	}

	mapped, err := mapping.Apply(source, route.ResponseMapping)
	if err != nil {
		return fmt.Errorf("apply response mapping: %w", err)
	}

	mappedBody, err := json.Marshal(mapped)
	if err != nil {
		return fmt.Errorf("encode mapped response: %w", err)
	}
	resp.Body = io.NopCloser(bytes.NewReader(mappedBody))
	resp.ContentLength = int64(len(mappedBody))
	resp.Header.Set("Content-Length", strconv.Itoa(len(mappedBody)))
	resp.Header.Set("Content-Type", "application/json; charset=utf-8")
	return nil
}

func isJSONResponse(resp *http.Response) bool {
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	return strings.Contains(contentType, "json")
}

func (g *Gateway) handleRedirect(w http.ResponseWriter, r *http.Request, route *router.Route) (string, string) {
	target, ok := route.RedirectPicker.Next()
	if !ok {
		http.Error(w, "no redirect target available", http.StatusBadGateway)
		return "", "no redirect target available"
	}

	statusCode := http.StatusFound
	if route.Config.Redirect != nil && route.Config.Redirect.StatusCode > 0 {
		statusCode = route.Config.Redirect.StatusCode
	}

	http.Redirect(w, r, target.URL, statusCode)
	return target.URL, ""
}

func (g *Gateway) logRequest(enabled bool, r *http.Request, routeName string, upstreamURL string, status int, duration time.Duration, errText string) {
	if !enabled {
		return
	}
	durationMS := duration.Milliseconds()
	attrs := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"route", routeName,
		"upstream", upstreamURL,
		"status", status,
		"duration_ms", durationMS,
		"client_ip", r.RemoteAddr,
	}
	if errText != "" {
		attrs = append(attrs, "error", errText)
	}
	g.logger.Info("request handled", attrs...)

	if g.logStore == nil {
		return
	}
	log := store.RequestLog{
		RequestID:   g.nextRequestID(),
		Method:      r.Method,
		Path:        requestPath(r),
		RouteID:     routeName,
		UpstreamURL: upstreamURL,
		StatusCode:  status,
		DurationMS:  durationMS,
		ClientIP:    clientIP(r),
		Error:       errText,
	}
	if _, err := g.logStore.CreateRequestLog(context.Background(), log); err != nil {
		g.logger.Warn("persist request log failed", "error", err)
	}
}

func (g *Gateway) nextRequestID() string {
	return fmt.Sprintf("req_%d", atomic.AddUint64(&g.requestSeq, 1))
}

func requestPath(r *http.Request) string {
	if r.URL == nil {
		return ""
	}
	return r.URL.RequestURI()
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
