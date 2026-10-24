//go:build !apidocs

package companion

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Without -tags apidocs (release builds) the explorer is not served or linked.
func TestReleaseBuildsLeaveTheExplorerOut(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/docs", "/docs/swagger-ui-bundle.js", "/openapi.json"} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:9123"+path, nil)
		r.RemoteAddr = "127.0.0.1:3210"
		w := httptest.NewRecorder()
		s.http.Handler.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	s.renderSettings(w, "")
	if strings.Contains(w.Body.String(), "href=\"/docs\"") {
		t.Fatal("explorer linked in a build without it")
	}
}
