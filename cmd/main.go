package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mohammedbilalns/torra-tui/internal/downloader"
	"github.com/mohammedbilalns/torra-tui/internal/history"
	"github.com/mohammedbilalns/torra-tui/internal/tui"
	"github.com/mohammedbilalns/torra-tui/internal/tui/config"
)

func main() {
	configPath := getenv("TORR_CONFIG_PATH", defaultConfigPath())
	cfg, _ := config.Load(configPath)

	downloadDir := getenv("TORR_DOWNLOAD_DIR", cfg.DownloadDir)
	dbPath := getenv("TORR_DB_PATH", defaultDBPath())
	if downloadDir != "" && cfg.DownloadDir == "" {
		cfg.DownloadDir = downloadDir
	}
	if cfg.MaxParallelDownloads < 0 {
		cfg.MaxParallelDownloads = 0
	}
	if cfg.DownloadRateLimitKbps < 0 {
		cfg.DownloadRateLimitKbps = 0
	}
	if cfg.UploadRateLimitKbps < 0 {
		cfg.UploadRateLimitKbps = 0
	}
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		_ = config.Save(configPath, cfg)
	}

	if downloadDir != "" {
		if err := os.MkdirAll(downloadDir, 0o755); err != nil {
			fmt.Printf("Failed to create download dir: %v\n", err)
			os.Exit(1)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		fmt.Printf("Failed to create db dir: %v\n", err)
		os.Exit(1)
	}

	store, err := history.Open(dbPath)
	if err != nil {
		fmt.Printf("Failed to open db: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	var manager *downloader.Manager
	if downloadDir != "" {
		manager, err = downloader.NewManager(downloader.Config{
			DownloadDir:           downloadDir,
			DownloadRateLimitKbps: cfg.DownloadRateLimitKbps,
			UploadRateLimitKbps:   cfg.UploadRateLimitKbps,
		})
		if err != nil {
			fmt.Printf("Failed to start torrent client: %v\n", err)
			os.Exit(1)
		}
		defer manager.Close()
	}

	model, err := tui.NewModel(store, manager, downloadDir, configPath, dbPath, cfg)
	if err != nil {
		fmt.Printf("Failed to load model: %v\n", err)
		os.Exit(1)
	}

	if err := tea.NewProgram(model, tea.WithAltScreen()).Start(); err != nil {
		fmt.Printf("Program error: %v\n", err)
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "./config.toml"
	}
	return filepath.Join(dir, "torra-tui", "config.toml")
}

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "./torr.db"
	}
	return filepath.Join(home, ".local", "share", "torra-tui", "torr.db")
}
