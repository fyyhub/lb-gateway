package health

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"light-api-gateway/internal/config"
)

const (
	StatusHealthy   = "healthy"
	StatusUnhealthy = "unhealthy"
	StatusUnknown   = "unknown"
)

type Store interface {
	ListAllUpstreamTargets(ctx context.Context) ([]config.TargetConfig, error)
	SetUpstreamTargetHealthStatus(ctx context.Context, id string, status string) (config.TargetConfig, error)
}

type Checker struct {
	store    Store
	client   *http.Client
	interval time.Duration
	logger   *slog.Logger
}

type Option func(*Checker)

func NewChecker(store Store, logger *slog.Logger, opts ...Option) *Checker {
	checker := &Checker{
		store:    store,
		client:   &http.Client{Timeout: 2 * time.Second},
		interval: 10 * time.Second,
		logger:   logger,
	}
	if checker.logger == nil {
		checker.logger = slog.Default()
	}
	for _, opt := range opts {
		opt(checker)
	}
	if checker.client == nil {
		checker.client = &http.Client{Timeout: 2 * time.Second}
	}
	if checker.interval <= 0 {
		checker.interval = 10 * time.Second
	}
	return checker
}

func WithClient(client *http.Client) Option {
	return func(checker *Checker) {
		checker.client = client
	}
}

func WithInterval(interval time.Duration) Option {
	return func(checker *Checker) {
		checker.interval = interval
	}
}

func (c *Checker) Run(ctx context.Context) {
	c.CheckOnce(ctx)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.CheckOnce(ctx)
		}
	}
}

func (c *Checker) CheckOnce(ctx context.Context) {
	targets, err := c.store.ListAllUpstreamTargets(ctx)
	if err != nil {
		c.logger.Warn("list upstream targets for health check failed", "error", err)
		return
	}

	for _, target := range targets {
		if ctx.Err() != nil {
			return
		}
		if !target.Enabled || target.ID == "" || strings.TrimSpace(target.URL) == "" {
			continue
		}

		status, err := c.probe(ctx, target.URL)
		if err != nil {
			c.logger.Warn("upstream health check failed", "target_id", target.ID, "url", target.URL, "error", err)
		}
		if target.HealthStatus == status {
			continue
		}

		if _, updateErr := c.store.SetUpstreamTargetHealthStatus(ctx, target.ID, status); updateErr != nil {
			c.logger.Warn("update upstream health status failed", "target_id", target.ID, "status", status, "error", updateErr)
			continue
		}
		c.logger.Info("upstream health status changed", "target_id", target.ID, "url", target.URL, "status", status)
	}
}

func (c *Checker) probe(ctx context.Context, rawURL string) (string, error) {
	targetURL, err := url.Parse(rawURL)
	if err != nil {
		return StatusUnhealthy, fmt.Errorf("parse target url: %w", err)
	}
	if targetURL.Scheme != "http" && targetURL.Scheme != "https" {
		return StatusUnhealthy, fmt.Errorf("unsupported target scheme %q", targetURL.Scheme)
	}

	statusCode, err := c.request(ctx, http.MethodHead, targetURL.String())
	if err != nil {
		return StatusUnhealthy, err
	}
	if statusCode == http.StatusMethodNotAllowed {
		statusCode, err = c.request(ctx, http.MethodGet, targetURL.String())
		if err != nil {
			return StatusUnhealthy, err
		}
	}

	if statusCode >= 500 {
		return StatusUnhealthy, fmt.Errorf("upstream returned status %d", statusCode)
	}
	return StatusHealthy, nil
}

func (c *Checker) request(ctx context.Context, method string, rawURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create health request: %w", err)
	}
	req.Header.Set("User-Agent", "light-api-gateway-health-check")

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send health request: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))

	return resp.StatusCode, nil
}
