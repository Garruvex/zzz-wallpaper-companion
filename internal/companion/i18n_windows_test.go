//go:build windows

package companion

import "testing"

func TestWindowsDisplayLanguageMapsToTheCompanionLanguages(t *testing.T) {
	for id, want := range map[uint16]string{
		0x0409: "en",    // English (US)
		0x0809: "en",    // English (UK)
		0x0411: "en",    // Japanese: not offered
		0x0804: "zh-CN", // Chinese (Simplified, China)
		0x1004: "zh-CN", // Chinese (Simplified, Singapore)
		0x0404: "zh-TW", // Chinese (Traditional, Taiwan)
		0x0c04: "zh-TW", // Chinese (Traditional, Hong Kong)
		0x1404: "zh-TW", // Chinese (Traditional, Macao)
		0x7c04: "zh-TW", // Chinese (Traditional)
	} {
		if got := languageForLangID(id); got != want {
			t.Errorf("LANGID %#x: got %s, want %s", id, got, want)
		}
	}
}
