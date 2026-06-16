package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Gateway GatewayConfig `json:"gateway"`
	Routes  []RouteConfig `json:"routes"`
}

type GatewayConfig struct {
	Listen              string `json:"listen"`
	ReadTimeoutSeconds  int    `json:"readTimeoutSeconds"`
	WriteTimeoutSeconds int    `json:"writeTimeoutSeconds"`
	IdleTimeoutSeconds  int    `json:"idleTimeoutSeconds"`
	LogRequests         bool   `json:"logRequests"`
}

func (c GatewayConfig) ListenOrDefault() string {
	if c.Listen == "" {
		return ":8080"
	}
	return c.Listen
}

type RouteConfig struct {
	ID               string          `json:"id,omitempty"`
	Name             string          `json:"name"`
	Enabled          bool            `json:"enabled"`
	Priority         int             `json:"priority"`
	Type             string          `json:"type"`
	Match            MatchConfig     `json:"match"`
	UpstreamGroupID  string          `json:"upstreamGroupId,omitempty"`
	UpstreamGroup    UpstreamGroup   `json:"upstreamGroup,omitempty"`
	RequestRewrite   []RewriteRule   `json:"requestRewrite,omitempty"`
	ResponseMapping  []MappingRule   `json:"responseMapping,omitempty"`
	Redirect         *RedirectConfig `json:"redirect,omitempty"`
	MaxResponseBytes int64           `json:"maxResponseBytes,omitempty"`
}

type MatchConfig struct {
	Host    string   `json:"host,omitempty"`
	Path    string   `json:"path"`
	Methods []string `json:"methods,omitempty"`
}

type UpstreamGroup struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name,omitempty"`
	Strategy string         `json:"strategy"`
	Targets  []TargetConfig `json:"targets"`
}

type RedirectConfig struct {
	StatusCode int            `json:"statusCode"`
	Strategy   string         `json:"strategy"`
	Targets    []TargetConfig `json:"targets"`
}

type TargetConfig struct {
	ID           string `json:"id,omitempty"`
	GroupID      string `json:"groupId,omitempty"`
	URL          string `json:"url"`
	Weight       int    `json:"weight"`
	Enabled      bool   `json:"enabled"`
	HealthStatus string `json:"healthStatus,omitempty"`
}

type RewriteRule struct {
	Type  string `json:"type"`
	Key   string `json:"key,omitempty"`
	Value any    `json:"value,omitempty"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

type MappingRule struct {
	From  string `json:"from,omitempty"`
	To    string `json:"to"`
	Value any    `json:"value,omitempty"`
}

func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	var cfg Config
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	if cfg.Gateway.Listen == "" {
		cfg.Gateway.Listen = ":8080"
	}

	return cfg, nil
}
