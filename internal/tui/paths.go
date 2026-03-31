package tui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	testFile := filepath.Join(dir, ".torra-tui-write-test")
	if err := os.WriteFile(testFile, []byte("ok"), 0o644); err != nil {
		return err
	}
	return os.Remove(testFile)
}

func expandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", fmt.Errorf("cannot resolve home directory")
		}
		if path == "~" {
			path = home
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, path[2:])
		}
	}
	return filepath.Clean(path), nil
}

func isValidMagnet(magnet string) bool {
	u, err := url.Parse(magnet)
	if err != nil {
		return false
	}
	if u.Scheme != "magnet" {
		return false
	}
	if u.RawQuery == "" {
		return false
	}
	q := u.Query()
	xts := q["xt"]
	for _, xt := range xts {
		if strings.HasPrefix(xt, "urn:btih:") || strings.HasPrefix(xt, "urn:btmh:") {
			return true
		}
	}
	return false
}

func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".mp4", ".mkv", ".avi", ".mov", ".webm", ".m4v", ".mpg", ".mpeg", ".flv", ".wmv", ".3gp", ".ts":
		return true
	default:
		return false
	}
}

func minPlayableBytes(total int64) int64 {
	if total <= 0 {
		return 256 * 1024
	}
	min := int64(256 * 1024)
	if total < min {
		return total
	}
	if total > 5*1024*1024 {
		return 1024 * 1024
	}
	return min
}
