package companion

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Opt-in real wallpaper adapter + core + API integration; all imports and
// writes go to testServer's temporary data directory, never the real library.
func TestEmulatorBrowserIntegration(t *testing.T) {
	if os.Getenv("COMPANION_EMULATOR_BROWSER") != "1" {
		t.Skip("opt-in emulator browser integration")
	}
	wallpaper := os.Getenv("COMPANION_WALLPAPER_PATH")
	if wallpaper == "" {
		t.Fatal("set COMPANION_WALLPAPER_PATH")
	}
	server := testServer(t)
	host := httptest.NewServer(server.http.Handler)
	defer host.Close()
	cmd := exec.Command("node", filepath.Join(wallpaper, "tools", "emulator-smoke.cjs"))
	cmd.Dir = wallpaper
	cmd.Env = append(os.Environ(), "EMULATOR_COMPANION_URL="+host.URL)
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatal(err)
	}
}
