package tui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"

	"github.com/mohammedbilalns/torra-cli/internal/downloader"
	"github.com/mohammedbilalns/torra-cli/internal/history"
	"github.com/mohammedbilalns/torra-cli/internal/models"
	"github.com/mohammedbilalns/torra-cli/internal/tui/config"
)

type mode int

const (
	modeList mode = iota
	modeAdd
	modeConfirmDelete
	modeSetupDir
	modeChangeDir
	modeConfirmReset
)

type tickMsg time.Time

type Model struct {
	store       *history.Store
	downloadDir string
	configPath  string
	dbPath      string
	cfg         config.Config
	managers    map[string]*downloader.Manager
	taskManager map[string]*downloader.Manager

	tasks    []models.TorrentEntry
	selected int
	mode     mode
	input    textinput.Model
	status   string
	statusAt time.Time
	width    int
	height   int

	lastBytes map[string]int64
	lastTime  map[string]time.Time
	speeds    map[string]float64
}

func NewModel(store *history.Store, manager *downloader.Manager, downloadDir, configPath, dbPath string, cfg config.Config) (Model, error) {
	entries, err := store.List()
	if err != nil {
		return Model{}, err
	}
	input := textinput.New()
	input.Placeholder = "magnet:?xt=urn:btih:..."
	input.Focus()
	input.CharLimit = 0
	input.Width = 60

	startMode := modeList
	if downloadDir == "" {
		startMode = modeSetupDir
		input.Placeholder = "/path/to/downloads"
	}

	managers := make(map[string]*downloader.Manager)
	if manager != nil && downloadDir != "" {
		managers[downloadDir] = manager
	}

	return Model{
		store:       store,
		downloadDir: downloadDir,
		configPath:  configPath,
		dbPath:      dbPath,
		cfg:         cfg,
		managers:    managers,
		taskManager: make(map[string]*downloader.Manager),
		tasks:       entries,
		mode:        startMode,
		input:       input,
		lastBytes:   make(map[string]int64),
		lastTime:    make(map[string]time.Time),
		speeds:      make(map[string]float64),
	}, nil
}

func (m Model) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(700*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.mode == modeAdd || m.mode == modeSetupDir || m.mode == modeChangeDir {
			switch msg.String() {
			case "esc":
				if m.mode == modeSetupDir {
					return m, nil
				}
				m.mode = modeList
				m.input.SetValue("")
				return m, nil
			case "enter":
				if m.mode == modeAdd {
					magnet := strings.TrimSpace(m.input.Value())
					m.input.SetValue("")
					if magnet == "" {
						m.status = "Magnet link is empty."
						return m, nil
					}
					if !isValidMagnet(magnet) {
						m.status = "Invalid magnet link."
						return m, nil
					}
					if m.hasDuplicateMagnet(magnet) {
						m.status = "Duplicate magnet already exists."
						m.mode = modeList
						return m, nil
					}
					if m.downloadDir == "" {
						m.status = "Set a download directory first."
						m.mode = modeSetupDir
						return m, nil
					}
					if m.cfg.MaxParallelDownloads > 0 && m.activeDownloadsCount() >= m.cfg.MaxParallelDownloads {
						m.status = fmt.Sprintf("Max parallel downloads reached (%d).", m.cfg.MaxParallelDownloads)
						return m, nil
					}
					mgr, err := m.managerForDir(m.downloadDir)
					if err != nil {
						m.status = fmt.Sprintf("Failed to create client: %v", err)
						return m, nil
					}
					entry := models.TorrentEntry{
						ID:          uuid.NewString(),
						Magnet:      magnet,
						Name:        "Fetching metadata...",
						DownloadDir: m.downloadDir,
						State:       "downloading",
						Completed:   false,
						CreatedAt:   time.Now(),
					}
					if _, err := mgr.Start(entry.ID, entry.Magnet, entry.DownloadDir); err != nil {
						m.status = fmt.Sprintf("Start failed: %v", err)
						return m, nil
					}
					m.taskManager[entry.ID] = mgr
					_ = m.store.Upsert(entry)
					m.tasks = append([]models.TorrentEntry{entry}, m.tasks...)
					m.mode = modeList
					m.status = "Started download."
					m.statusAt = time.Now()
					m.lastBytes[entry.ID] = 0
					m.lastTime[entry.ID] = time.Now()
					m.speeds[entry.ID] = 0
					return m, nil
				}
				if m.mode == modeSetupDir || m.mode == modeChangeDir {
					dir := strings.TrimSpace(m.input.Value())
					if dir == "" {
						m.status = "Directory is empty."
						return m, nil
					}
					expanded, err := expandPath(dir)
					if err != nil {
						m.status = fmt.Sprintf("Invalid directory: %v", err)
						return m, nil
					}
					if err := ensureWritableDir(expanded); err != nil {
						m.status = fmt.Sprintf("Invalid directory: %v", err)
						return m, nil
					}
					if _, err := m.managerForDir(expanded); err != nil {
						m.status = fmt.Sprintf("Failed to create client: %v", err)
						return m, nil
					}
					m.downloadDir = expanded
					m.cfg.DownloadDir = expanded
					_ = config.Save(m.configPath, m.cfg)
					m.input.SetValue("")
					m.mode = modeList
					m.status = "Download directory set."
					m.statusAt = time.Now()
					return m, nil
				}
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		if m.mode == modeConfirmDelete {
			switch msg.String() {
			case "y", "Y":
				if err := m.deleteSelected(); err != nil {
					m.status = fmt.Sprintf("Delete failed: %v", err)
				} else {
					m.status = "Deleted."
					m.statusAt = time.Now()
				}
				m.mode = modeList
				return m, nil
			case "n", "N", "esc":
				m.mode = modeList
				m.status = "Delete cancelled."
				m.statusAt = time.Now()
				return m, nil
			}
			return m, nil
		}
		if m.mode == modeConfirmReset {
			switch msg.String() {
			case "y", "Y":
				if err := m.resetAll(); err != nil {
					m.status = fmt.Sprintf("Reset failed: %v", err)
					m.mode = modeList
					return m, nil
				}
				return m, tea.Quit
			case "n", "N", "esc":
				m.mode = modeList
				m.status = "Reset cancelled."
				m.statusAt = time.Now()
				return m, nil
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "a":
			if m.downloadDir == "" {
				m.mode = modeSetupDir
				m.input.Placeholder = "/path/to/downloads"
				m.input.Focus()
				return m, nil
			}
			m.mode = modeAdd
			m.input.Placeholder = "magnet:?xt=urn:btih:..."
			m.input.Focus()
			return m, nil
		case "d":
			if m.current() != nil {
				m.mode = modeConfirmDelete
				m.status = ""
			}
			return m, nil
		case "c":
			m.mode = modeChangeDir
			m.input.SetValue(m.downloadDir)
			m.input.Placeholder = "/path/to/downloads"
			m.input.Focus()
			return m, nil
		case "x":
			m.mode = modeConfirmReset
			m.status = ""
			return m, nil
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.tasks)-1 {
				m.selected++
			}
		case "s":
			entry := m.current()
			if entry == nil {
				return m, nil
			}
			mgr := m.taskManager[entry.ID]
			if mgr == nil {
				m.status = "Not active in this session."
				m.statusAt = time.Now()
				entry.State = "paused"
				_ = m.store.Upsert(*entry)
				return m, nil
			}
			mgr.Pause(entry.ID)
			entry.State = "paused"
			_ = m.store.Upsert(*entry)
			m.status = "Paused."
			m.statusAt = time.Now()
		case "r":
			entry := m.current()
			if entry == nil {
				return m, nil
			}
			if m.cfg.MaxParallelDownloads > 0 && m.activeDownloadsCount() >= m.cfg.MaxParallelDownloads {
				m.status = fmt.Sprintf("Max parallel downloads reached (%d).", m.cfg.MaxParallelDownloads)
				m.statusAt = time.Now()
				return m, nil
			}
			mgr, err := m.managerForDir(entry.DownloadDir)
			if err != nil {
				m.status = fmt.Sprintf("Failed to create client: %v", err)
				m.statusAt = time.Now()
				return m, nil
			}
			if mgr.Get(entry.ID) == nil {
				if _, err := mgr.Start(entry.ID, entry.Magnet, entry.DownloadDir); err != nil {
					m.status = fmt.Sprintf("Resume failed: %v", err)
					m.statusAt = time.Now()
					return m, nil
				}
			} else if _, err := mgr.Resume(entry.ID); err != nil {
				m.status = fmt.Sprintf("Resume failed: %v", err)
				m.statusAt = time.Now()
				return m, nil
			}
			m.taskManager[entry.ID] = mgr
			entry.State = "downloading"
			_ = m.store.Upsert(*entry)
			m.status = "Resumed."
			m.statusAt = time.Now()
		}
	case tickMsg:
		if m.status != "" && time.Since(m.statusAt) > 5*time.Second {
			m.status = ""
		}
		updated := false
		for i := range m.tasks {
			entry := &m.tasks[i]
			if entry.State != "downloading" {
				m.speeds[entry.ID] = 0
				continue
			}
			mgr := m.taskManager[entry.ID]
			if mgr == nil {
				m.speeds[entry.ID] = 0
				continue
			}
			completed, length, name := mgr.BytesCompleted(entry.ID)
			now := time.Now()
			if last, ok := m.lastTime[entry.ID]; ok {
				elapsed := now.Sub(last).Seconds()
				if elapsed > 0 {
					prev := m.lastBytes[entry.ID]
					delta := completed - prev
					if delta < 0 {
						delta = 0
					}
					m.speeds[entry.ID] = float64(delta) / elapsed
				}
			}
			m.lastBytes[entry.ID] = completed
			m.lastTime[entry.ID] = now
			if length > 0 {
				entry.Length = length
				entry.BytesCompleted = completed
			}
			if name != "" && entry.Name != name {
				entry.Name = name
			}
			if entry.Length > 0 && entry.BytesCompleted >= entry.Length {
				entry.Completed = true
				entry.State = "completed"
			}
			_ = m.store.Upsert(*entry)
			updated = true
		}
		if updated {
			return m, tickCmd()
		}
		return m, tickCmd()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width < 20 {
			m.width = 20
		}
		if m.height < 5 {
			m.height = 5
		}
		if m.width > 10 {
			m.input.Width = m.width - 10
		}
	}
	return m, nil
}

func (m Model) View() string {
	frame := lipgloss.NewStyle().Padding(1, 2)
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8EDEB")).Render("TORR CLI")
	sub := lipgloss.NewStyle().Foreground(lipgloss.Color("#C8D7E6")).Render("a:add r:resume s:pause d:delete c:change dir x:reset q:quit")
	top := header + "\n" + sub

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6C8EAD")).
		Padding(1, 2)

	status := ""
	if m.status != "" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("#E6B566")).Render(m.status)
	}

	if m.mode == modeAdd {
		content := "Add magnet link:\n" + m.input.View() + "\n\nPress Enter to start, Esc to cancel."
		return m.fullscreen(frame.Render(top + "\n\n" + box.Render(content) + "\n\n" + status))
	}
	if m.mode == modeConfirmDelete {
		content := "Delete selected torrent and files?\n\nPress y to confirm, n to cancel."
		return m.fullscreen(frame.Render(top + "\n\n" + box.Render(content) + "\n\n" + status))
	}
	if m.mode == modeConfirmReset {
		content := "Reset everything (DB + config + downloaded files)?\n\nPress y to confirm, n to cancel."
		return m.fullscreen(frame.Render(top + "\n\n" + box.Render(content) + "\n\n" + status))
	}
	if m.mode == modeSetupDir {
		content := "Choose a download directory:\n" + m.input.View() + "\n\nPress Enter to save."
		return m.fullscreen(frame.Render(top + "\n\n" + box.Render(content) + "\n\n" + status))
	}
	if m.mode == modeChangeDir {
		content := "Change download directory:\n" + m.input.View() + "\n\nPress Enter to save, Esc to cancel."
		return m.fullscreen(frame.Render(top + "\n\n" + box.Render(content) + "\n\n" + status))
	}

	if len(m.tasks) == 0 {
		content := "No torrents yet. Press 'a' to add one."
		return m.fullscreen(frame.Render(top + "\n\n" + box.Render(content) + "\n\n" + status))
	}

	var lines []string
	for i, entry := range m.tasks {
		cursor := " "
		if i == m.selected {
			cursor = "›"
		}
		name := entry.Name
		if name == "" {
			name = entry.Magnet
		}
		percent := "--"
		if entry.Length > 0 {
			percent = fmt.Sprintf("%.1f%%", (float64(entry.BytesCompleted)/float64(entry.Length))*100)
		}
		speed := formatSpeed(m.speeds[entry.ID])
		line := fmt.Sprintf("%s [%s] %s (%s) %s/%s",
			cursor,
			entry.State,
			trimTo(name, 48),
			percent,
			formatBytes(entry.BytesCompleted),
			formatBytes(entry.Length),
		)
		if speed != "" {
			line += "  " + speed
		}
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return m.fullscreen(frame.Render(top + "\n\n" + box.Render(content) + "\n\n" + status))
}

func (m *Model) current() *models.TorrentEntry {
	if len(m.tasks) == 0 || m.selected < 0 || m.selected >= len(m.tasks) {
		return nil
	}
	return &m.tasks[m.selected]
}

func trimTo(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func formatBytes(v int64) string {
	if v <= 0 {
		return "0 B"
	}
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%d B", v)
	}
	div, exp := int64(unit), 0
	for n := v / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	value := float64(v) / float64(div)
	return fmt.Sprintf("%.1f %cB", value, "KMGTPE"[exp])
}

func formatSpeed(v float64) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprintf("↓ %s/s", formatBytes(int64(v)))
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

func (m Model) fullscreen(s string) string {
	if m.width == 0 || m.height == 0 {
		return s
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > m.height {
		lines = lines[:m.height]
	}
	for i := range lines {
		lineLen := lipgloss.Width(lines[i])
		if lineLen < m.width {
			lines[i] += strings.Repeat(" ", m.width-lineLen)
		} else if lineLen > m.width {
			lines[i] = lipgloss.NewStyle().MaxWidth(m.width).Render(lines[i])
		}
	}
	for len(lines) < m.height {
		lines = append(lines, strings.Repeat(" ", m.width))
	}
	return strings.Join(lines, "\n")
}

func (m Model) activeDownloadsCount() int {
	count := 0
	for _, entry := range m.tasks {
		if entry.State == "downloading" && m.taskManager[entry.ID] != nil {
			count++
		}
	}
	return count
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

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	testFile := filepath.Join(dir, ".torr-cli-write-test")
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
