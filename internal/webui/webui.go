// Package webui embeds the built React admin UI and serves it (with SPA
// fallback) so the whole product can run from a single Go binary and a single
// port, without a separate nginx container.
//
// The admin UI is built by Vite with base path "/admin/", so all of its asset
// URLs live under /admin/. The build output is copied into the dist directory
// before compiling the binary (the Dockerfile and local build do this). When
// the UI has not been built, a small placeholder page is served instead so the
// binary still compiles and runs.
package webui

import (
	"bytes"
	"embed"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

//go:embed all:dist
var distFS embed.FS

// Mount is the URL prefix the admin UI is served under.
const Mount = "/admin"

// Handler returns an http.Handler that serves the embedded admin UI under the
// Mount prefix, falling back to index.html for unknown paths. The returned
// handler expects to receive full request paths (it strips the Mount prefix
// internally).
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return notBuilt("admin UI assets are unavailable")
	}
	indexHTML, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return notBuilt("admin UI has not been built; build web-admin before compiling the binary")
	}

	modTime := time.Now()
	fileServer := http.FileServer(http.FS(sub))

	serveIndex := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", modTime, bytes.NewReader(indexHTML))
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/")
		if rel == "" {
			serveIndex(w, r)
			return
		}
		if info, statErr := fs.Stat(sub, rel); statErr == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Unknown path or directory: fall back to the SPA entry point.
		serveIndex(w, r)
	})

	return http.StripPrefix(Mount, inner)
}

func notBuilt(message string) http.Handler {
	page := []byte("<!doctype html><html lang=\"en\"><meta charset=\"utf-8\">" +
		"<title>Light API Gateway</title>" +
		"<body style=\"font-family:system-ui,sans-serif;background:#0b1120;color:#e6edf7;display:grid;place-items:center;min-height:100vh;margin:0\">" +
		"<div style=\"text-align:center;max-width:520px;padding:24px\">" +
		"<h1 style=\"margin:0 0 8px\">Admin UI not built</h1>" +
		"<p style=\"color:#94a5be;line-height:1.6\">" + message + "</p>" +
		"</div></body></html>")
	return http.StripPrefix(Mount, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(page)
	}))
}
