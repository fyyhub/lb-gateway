package rewrite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"light-api-gateway/internal/config"
	"light-api-gateway/internal/mapping"
)

const maxRewriteBodyBytes = 1 << 20

func Apply(req *http.Request, rules []config.RewriteRule) error {
	for _, rule := range rules {
		switch rule.Type {
		case "setHeader":
			if rule.Key == "" {
				return fmt.Errorf("setHeader rule requires key")
			}
			req.Header.Set(rule.Key, stringValue(rule.Value))
		case "setQuery":
			if rule.Key == "" {
				return fmt.Errorf("setQuery rule requires key")
			}
			query := req.URL.Query()
			query.Set(rule.Key, stringValue(rule.Value))
			req.URL.RawQuery = query.Encode()
		case "rewritePath":
			if rule.From == "" {
				return fmt.Errorf("rewritePath rule requires from")
			}
			if strings.HasPrefix(req.URL.Path, rule.From) {
				req.URL.Path = rule.To + strings.TrimPrefix(req.URL.Path, rule.From)
				if req.URL.Path == "" {
					req.URL.Path = "/"
				}
				req.URL.RawPath = ""
			}
		case "setJsonBody":
			if rule.Key == "" {
				return fmt.Errorf("setJsonBody rule requires key")
			}
			if err := setJSONBody(req, rule.Key, rule.Value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported rewrite rule type: %s", rule.Type)
		}
	}
	return nil
}

func setJSONBody(req *http.Request, path string, value any) error {
	var root any = map[string]any{}
	if req.Body != nil && req.Body != http.NoBody {
		body, err := io.ReadAll(io.LimitReader(req.Body, maxRewriteBodyBytes+1))
		if err != nil {
			return fmt.Errorf("read request body: %w", err)
		}
		req.Body.Close()
		if len(body) > maxRewriteBodyBytes {
			return fmt.Errorf("request body exceeds rewrite limit")
		}
		if len(bytes.TrimSpace(body)) > 0 {
			if err := json.Unmarshal(body, &root); err != nil {
				return fmt.Errorf("decode json request body: %w", err)
			}
		}
	}

	updated, err := mapping.Set(root, path, value)
	if err != nil {
		return err
	}

	body, err := json.Marshal(updated)
	if err != nil {
		return fmt.Errorf("encode json request body: %w", err)
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return nil
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}
