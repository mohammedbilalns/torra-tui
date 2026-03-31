package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"github.com/mohammedbilalns/torra-tui/internal/downloader"
	"github.com/mohammedbilalns/torra-tui/internal/history"
	"github.com/mohammedbilalns/torra-tui/internal/models"
	"github.com/mohammedbilalns/torra-tui/internal/tui/config"
)

type mode int

const (
	modeList mode = iota
	modeAdd
	modeConfirmDelete
	modeSetupDir
	modeChangeDir
	modeConfirmReset
	modePromptPlay
	modeSelectVideo
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

	prompted     map[string]bool
	promptTaskID string
	videoFiles   []videoFile
	videoSelect  int
}

func (m *Model) clearVideoPrompt() {
	m.mode = modeList
	m.promptTaskID = ""
	m.videoFiles = nil
	m.videoSelect = 0
}

func (m *Model) tryPlayVideo(vf videoFile) {
	mgr := m.taskManager[m.promptTaskID]
	if mgr == nil {
		m.status = "Torrent not active in this session."
		m.statusAt = time.Now()
		return
	}
	reader, err := mgr.StreamFile(m.promptTaskID, vf.Path)
	if err != nil {
		m.status = err.Error()
		m.statusAt = time.Now()
		return
	}
	if err := playVideoStream(reader); err != nil {
		m.status = err.Error()
		m.statusAt = time.Now()
		_ = reader.Close()
		return
	}
	m.status = "Streaming to mpv..."
	m.statusAt = time.Now()
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
		prompted:    make(map[string]bool),
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
		if m.mode == modePromptPlay {
			switch msg.String() {
			case "p":
				if len(m.videoFiles) == 1 {
					m.tryPlayVideo(m.videoFiles[0])
					m.clearVideoPrompt()
					return m, nil
				}
				m.mode = modeSelectVideo
				m.videoSelect = 0
				return m, nil
			case "d", "esc":
				m.clearVideoPrompt()
				return m, nil
			}
			return m, nil
		}
		if m.mode == modeSelectVideo {
			switch msg.String() {
			case "up", "k":
				if m.videoSelect > 0 {
					m.videoSelect--
				}
				return m, nil
			case "down", "j":
				if m.videoSelect < len(m.videoFiles)-1 {
					m.videoSelect++
				}
				return m, nil
			case "enter":
				if len(m.videoFiles) > 0 {
					m.tryPlayVideo(m.videoFiles[m.videoSelect])
				}
				m.clearVideoPrompt()
				return m, nil
			case "esc":
				m.clearVideoPrompt()
				return m, nil
			}
			return m, nil
		}
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

			if entry.State == "downloading" && !m.prompted[entry.ID] {
				mgr := m.taskManager[entry.ID]
				if mgr == nil {
					continue
				}
				files, err := mgr.Files(entry.ID)
				if err != nil {
					if !errors.Is(err, downloader.ErrNoInfo) {
						m.prompted[entry.ID] = true
					}
					continue
				}
				videos := filterVideoFiles(files)
				m.prompted[entry.ID] = true
				if len(videos) > 0 {
					m.promptTaskID = entry.ID
					m.videoFiles = videos
					m.mode = modePromptPlay
					m.status = ""
					return m, tickCmd()
				}
			}
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
	return m.renderView()
}

func (m *Model) current() *models.TorrentEntry {
	if len(m.tasks) == 0 || m.selected < 0 || m.selected >= len(m.tasks) {
		return nil
	}
	return &m.tasks[m.selected]
}
