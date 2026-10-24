//go:build windows

package companion

import "syscall"

var procGetUserDefaultUILanguage = syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// systemLanguage is Windows' display language as one of the companion's languages.
func systemLanguage() string {
	id, _, _ := procGetUserDefaultUILanguage.Call()
	return languageForLangID(uint16(id))
}

// languageForLangID maps a Windows LANGID: Chinese (primary 0x04) in Taiwan, Hong Kong, Macao or
// Traditional script is Traditional; any other Chinese is Simplified; everything else is English.
func languageForLangID(id uint16) string {
	if id&0x3ff != 0x04 {
		return "en"
	}
	switch id {
	case 0x0404, 0x0c04, 0x1404, 0x7c04:
		return "zh-TW"
	default:
		return "zh-CN"
	}
}
