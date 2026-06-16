package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlerServesAdminPaths verifies the handler responds under the Mount
// prefix with HTML, whether or not the UI has been built into dist. It also
// confirms unknown paths fall back to the SPA entry point rather than 404.
func TestHandlerServesAdminPaths(t *testing.T) {
	handler := Handler()

	for _, path := range []string{Mount, Mount + "/", Mount + "/some/spa/route"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		// 200 when the UI is built and embedded, 503 when only the placeholder
		// is present; never a 404 or 500.
		if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("path %q got status %d, want 200 or 503", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Fatalf("path %q got content-type %q, want text/html", path, ct)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("path %q returned an empty body", path)
		}
	}
}
