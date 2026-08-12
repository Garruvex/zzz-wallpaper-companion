//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico ../../winres/icon.ico -arch amd64 -o rsrc_windows_amd64.syso
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico ../../winres/icon.ico -arch arm64 -o rsrc_windows_arm64.syso

package main

import "github.com/Garruvex/zzz-wallpaper-companion/internal/companion"

func main() {
	companion.Run()
}
