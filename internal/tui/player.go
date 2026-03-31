package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

func playVideo(path string) error {
	if _, err := exec.LookPath("mpv"); err != nil {
		return fmt.Errorf("mpv not found in PATH")
	}
	if runtime.GOOS != "windows" {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return fmt.Errorf("no graphical display found (DISPLAY/WAYLAND_DISPLAY not set)")
		}
	}
	cmd := exec.Command("mpv", "--force-window=yes", "--keep-open=yes", "--idle=yes", "--no-terminal", path)
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv: %w", err)
	}
	return nil
}

func playVideoStream(r io.ReadCloser) error {
	if _, err := exec.LookPath("mpv"); err != nil {
		return fmt.Errorf("mpv not found in PATH")
	}
	if runtime.GOOS != "windows" {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return fmt.Errorf("no graphical display found (DISPLAY/WAYLAND_DISPLAY not set)")
		}
	}
	cmd := exec.Command("mpv", "--force-window=yes", "--no-terminal", "--cache=yes", "--cache-secs=30", "-")
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	cmd.Stdin = r
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv: %w", err)
	}
	go func() {
		_ = cmd.Wait()
		_ = r.Close()
	}()
	return nil
}
