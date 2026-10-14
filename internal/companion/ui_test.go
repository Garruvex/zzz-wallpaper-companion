package companion

import (
	"net/http"
	"strings"
	"testing"
)

func TestSettingsInitialChannelIsJavaScriptString(t *testing.T) {
	response := cartRequest(t, testServer(t), http.MethodGet, "/settings", nil)
	if !strings.Contains(response.Body.String(), `updateChannel:"nightly"`) {
		t.Fatal("update channel was not rendered as a plain JS string")
	}
}
