package main

import (
	"encoding/json"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"
)

func main() {
	listen := flag.String("listen", ":9001", "listen address")
	name := flag.String("name", "mock-api-a", "service name")
	shape := flag.String("shape", "a", "response shape: a or b")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Mock-Service", *name)

		var payload any
		switch *shape {
		case "b":
			payload = map[string]any{
				"success": true,
				"service": *name,
				"path":    r.URL.Path,
				"result": map[string]any{
					"username": "Tom",
					"userId":   2,
				},
			}
		default:
			payload = map[string]any{
				"code":    0,
				"service": *name,
				"path":    r.URL.Path,
				"data": map[string]any{
					"name": "Tom",
					"id":   1,
				},
			}
		}

		if err := json.NewEncoder(w).Encode(payload); err != nil {
			logger.Error("write mock response failed", "error", err)
		}
	})

	server := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("mock api started", "listen", *listen, "name", *name, "shape", *shape)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("mock api stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}
