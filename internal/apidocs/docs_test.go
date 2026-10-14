package apidocs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestPinnedAssets(t *testing.T) {
	data, err := assets.ReadFile("assets/vendor.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string
		Files   map[string]struct{ SHA256 string }
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "5.33.1" || len(manifest.Files) != 4 {
		t.Fatal("incomplete pinned assets")
	}
	for name, item := range manifest.Files {
		data, err := assets.ReadFile("assets/" + name)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != item.SHA256 {
			t.Fatalf("asset drift: %s", name)
		}
	}
}
