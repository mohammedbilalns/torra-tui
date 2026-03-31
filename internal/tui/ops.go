package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mohammedbilalns/torra-tui/internal/downloader"
)

func (m Model) activeDownloadsCount() int {
	count := 0
	for _, entry := range m.tasks {
		if entry.State == "downloading" && m.taskManager[entry.ID] != nil {
			count++
		}
	}
	return count
}

func (m Model) hasDuplicateMagnet(magnet string) bool {
	for _, entry := range m.tasks {
		if strings.EqualFold(strings.TrimSpace(entry.Magnet), magnet) {
			return true
		}
	}
	return false
}

func (m *Model) deleteSelected() error {
	entry := m.current()
	if entry == nil {
		return nil
	}
	mgr := m.taskManager[entry.ID]
	if mgr == nil {
		mgr = m.managers[entry.DownloadDir]
	}
	if mgr != nil {
		mgr.Remove(entry.ID)
	}
	delete(m.taskManager, entry.ID)
	if entry.Name != "" && entry.Name != "Fetching metadata..." {
		target := filepath.Join(entry.DownloadDir, entry.Name)
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	}
	if err := m.store.Delete(entry.ID); err != nil {
		return err
	}
	idx := m.selected
	m.tasks = append(m.tasks[:idx], m.tasks[idx+1:]...)
	if m.selected >= len(m.tasks) && m.selected > 0 {
		m.selected--
	}
	return nil
}

func (m *Model) resetAll() error {
	for _, mgr := range m.managers {
		_ = mgr.Close()
	}
	m.managers = make(map[string]*downloader.Manager)
	m.taskManager = make(map[string]*downloader.Manager)
	for _, entry := range m.tasks {
		if entry.Name == "" || entry.Name == "Fetching metadata..." {
			continue
		}
		target := filepath.Join(entry.DownloadDir, entry.Name)
		_ = os.RemoveAll(target)
	}
	_ = m.store.Close()
	_ = os.Remove(m.dbPath)
	_ = os.Remove(m.configPath)
	return nil
}

func (m *Model) managerForDir(dir string) (*downloader.Manager, error) {
	if dir == "" {
		return nil, fmt.Errorf("empty download dir")
	}
	if mgr, ok := m.managers[dir]; ok && mgr != nil {
		return mgr, nil
	}
	mgr, err := downloader.NewManager(downloader.Config{
		DownloadDir:           dir,
		DownloadRateLimitKbps: m.cfg.DownloadRateLimitKbps,
		UploadRateLimitKbps:   m.cfg.UploadRateLimitKbps,
	})
	if err != nil {
		return nil, err
	}
	m.managers[dir] = mgr
	return mgr, nil
}

type videoFile struct {
	Path     string
	FullPath string
	Length   int64
}

func filterVideoFiles(files []downloader.FileInfo) []videoFile {
	var out []videoFile
	for _, f := range files {
		if isVideoFile(f.Path) {
			out = append(out, videoFile{
				Path:     f.Path,
				FullPath: f.FullPath,
				Length:   f.Length,
			})
		}
	}
	return out
}
