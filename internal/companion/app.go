//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico winres/icon.ico -arch amd64 -o rsrc_windows_amd64.syso
//go:generate go run github.com/akavel/rsrc@v0.10.2 -ico winres/icon.ico -arch arm64 -o rsrc_windows_arm64.syso

package companion

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	version     = "1.2.0"
	buildNumber = "dev"
)

func Run() {
	if len(os.Args) == 4 && os.Args[1] == "--apply-update" {
		if err := runUpdateHelper(os.Args[2], os.Args[3]); err != nil {
			fatalDialog("Companion update failed", err.Error())
		}
		return
	}
	confirmMarker := ""
	if len(os.Args) == 3 && os.Args[1] == "--confirm-update" {
		confirmMarker = os.Args[2]
	}
	dataDir, err := appDataDir()
	if err != nil {
		fatalDialog("ZZZ Wallpaper Companion", err.Error())
		return
	}
	releaseInstance, alreadyRunning, err := acquireSingleInstance()
	if err != nil {
		fatalDialog("Companion could not start", err.Error())
		return
	}
	if alreadyRunning {
		infoDialog("ZZZ Wallpaper Companion", "The companion is already running in the notification area.")
		return
	}
	defer releaseInstance()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		fatalDialog("ZZZ Wallpaper Companion", err.Error())
		return
	}
	if confirmMarker == "" && maybeLaunchPendingUpdate(dataDir) {
		return
	}
	logFile, err := os.OpenFile(filepath.Join(dataDir, "companion.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		defer logFile.Close()
		log.SetOutput(logFile)
	}
	log.Printf("starting ZZZ Wallpaper Companion %s (build %s)", version, buildNumber)

	config, err := newConfigStore(filepath.Join(dataDir, "settings.json"))
	if err != nil {
		fatalDialog("Invalid companion settings", err.Error())
		return
	}
	if config.Get().LaunchOnStartup {
		if err := setLaunchOnStartup(true); err != nil {
			log.Printf("refresh startup entry: %v", err)
		}
	}
	resolver := newResolver(dataDir, config)
	ffmpeg := newFFmpegManager(dataDir)
	updater := newUpdateManager(dataDir)
	server := newAPIServer(config, resolver, ffmpeg)
	quit := make(chan struct{})
	var quitOnce sync.Once
	requestQuit := func() { quitOnce.Do(func() { close(quit) }) }

	go func() {
		err := server.ListenAndServe(func() {
			if confirmMarker != "" {
				if err := os.WriteFile(confirmMarker, []byte("ready\n"), 0o600); err != nil {
					log.Printf("confirm update: %v", err)
				}
			}
		})
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server stopped: %v", err)
			fatalDialog("Companion could not start", err.Error())
			requestQuit()
		}
	}()

	go dependencyLoop(config, resolver, ffmpeg, updater, quit)
	go func() {
		if err := runTray(config, dataDir, resolver, updater, requestQuit); err != nil {
			log.Printf("tray unavailable: %v", err)
		}
	}()

	<-quit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	log.Print("companion stopped")
}

func appDataDir() (string, error) {
	if override := os.Getenv("ZZZ_COMPANION_DATA_DIR"); override != "" {
		return filepath.Abs(override)
	}
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		return "", errors.New("LOCALAPPDATA is not available")
	}
	return filepath.Join(root, "ZZZWallpaperCompanion"), nil
}

func dependencyLoop(config *ConfigStore, resolver *Resolver, ffmpeg *FFmpegManager, updater *UpdateManager, quit <-chan struct{}) {
	check := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
		defer cancel()
		if err := resolver.Update(ctx); err != nil {
			log.Printf("yt-dlp update: %v", err)
		} else {
			log.Print("yt-dlp is ready")
		}
		if err := ffmpeg.Ensure(ctx); err != nil {
			log.Printf("FFmpeg setup: %v", err)
		} else {
			log.Print("FFmpeg is ready")
		}
		if config.Get().AutoUpdate {
			result, err := updater.CheckAndStage(ctx)
			if err != nil {
				log.Printf("companion update: %v", err)
			} else if result.UpdateStaged {
				log.Printf("companion %s staged; it will install on next restart", result.LatestVersion)
			}
		}
	}
	check()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			check()
		case <-quit:
			return
		}
	}
}

func settingsURL(port int) string { return fmt.Sprintf("http://127.0.0.1:%d/settings", port) }
