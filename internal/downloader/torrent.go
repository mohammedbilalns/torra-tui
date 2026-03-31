package downloader

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"golang.org/x/time/rate"
)

type Task struct {
	ID          string
	Magnet      string
	Name        string
	DownloadDir string
	State       string

	torrent *torrent.Torrent
}

type Manager struct {
	client *torrent.Client
	mu     sync.Mutex
	tasks  map[string]*Task
}

type Config struct {
	DownloadDir           string
	DownloadRateLimitKbps int
	UploadRateLimitKbps   int
}

type FileInfo struct {
	Path     string
	FullPath string
	Length   int64
}

var ErrNoInfo = errors.New("torrent info not available")
var ErrFileNotFound = errors.New("file not found in torrent")

func NewManager(userCfg Config) (*Manager, error) {
	clientCfg := torrent.NewDefaultClientConfig()
	clientCfg.DataDir = userCfg.DownloadDir
	if userCfg.DownloadRateLimitKbps > 0 {
		bytesPerSec := userCfg.DownloadRateLimitKbps * 1024
		clientCfg.DownloadRateLimiter = rate.NewLimiter(rate.Limit(bytesPerSec), bytesPerSec)
	}
	if userCfg.UploadRateLimitKbps > 0 {
		bytesPerSec := userCfg.UploadRateLimitKbps * 1024
		clientCfg.UploadRateLimiter = rate.NewLimiter(rate.Limit(bytesPerSec), bytesPerSec)
	}
	client, err := torrent.NewClient(clientCfg)
	if err != nil {
		return nil, err
	}
	return &Manager{
		client: client,
		tasks:  make(map[string]*Task),
	}, nil
}

func (m *Manager) Close() error {
	if m == nil || m.client == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.client.Close()
	return nil
}

func (m *Manager) Start(id, magnet, downloadDir string) (*Task, error) {
	if magnet == "" {
		return nil, errors.New("empty magnet link")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	t, err := m.client.AddMagnet(magnet)
	if err != nil {
		return nil, err
	}
	task := &Task{
		ID:          id,
		Magnet:      magnet,
		DownloadDir: downloadDir,
		State:       "downloading",
		torrent:     t,
	}
	m.tasks[id] = task

	go func() {
		select {
		case <-t.GotInfo():
			t.DownloadAll()
		case <-time.After(10 * time.Second):
		}
	}()

	return task, nil
}

func (m *Manager) Pause(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task := m.tasks[id]
	if task == nil || task.torrent == nil {
		return
	}
	task.torrent.Drop()
	task.torrent = nil
	task.State = "paused"
}

func (m *Manager) Resume(id string) (*Task, error) {
	m.mu.Lock()
	task := m.tasks[id]
	m.mu.Unlock()
	if task == nil {
		return nil, errors.New("unknown task")
	}
	return m.Start(id, task.Magnet, task.DownloadDir)
}

func (m *Manager) Get(id string) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tasks[id]
}

func (m *Manager) Files(id string) ([]FileInfo, error) {
	m.mu.Lock()
	task := m.tasks[id]
	m.mu.Unlock()
	if task == nil {
		return nil, errors.New("unknown task")
	}
	if task.torrent == nil || task.torrent.Info() == nil {
		return nil, ErrNoInfo
	}
	info := task.torrent.Info()
	files := task.torrent.Files()
	baseName := strings.TrimSpace(info.BestName())
	out := make([]FileInfo, 0, len(files))
	for _, f := range files {
		path := f.Path()
		fullPath := filepath.Join(task.DownloadDir, filepath.FromSlash(path))
		if baseName != "" {
			if strings.HasPrefix(path, baseName+string('/')) || path == baseName {
				fullPath = filepath.Join(task.DownloadDir, filepath.FromSlash(path))
			} else {
				fullPath = filepath.Join(task.DownloadDir, filepath.FromSlash(filepath.Join(baseName, path)))
			}
		}
		out = append(out, FileInfo{
			Path:     path,
			FullPath: fullPath,
			Length:   f.Length(),
		})
	}
	return out, nil
}

func (m *Manager) DownloadFile(id, path string) error {
	m.mu.Lock()
	task := m.tasks[id]
	m.mu.Unlock()
	if task == nil {
		return errors.New("unknown task")
	}
	if task.torrent == nil || task.torrent.Info() == nil {
		return ErrNoInfo
	}
	for _, f := range task.torrent.Files() {
		if f.Path() == path {
			f.Download()
			return nil
		}
	}
	return ErrFileNotFound
}

func (m *Manager) StreamFile(id, path string) (torrent.Reader, error) {
	m.mu.Lock()
	task := m.tasks[id]
	m.mu.Unlock()
	if task == nil {
		return nil, errors.New("unknown task")
	}
	if task.torrent == nil || task.torrent.Info() == nil {
		return nil, ErrNoInfo
	}
	for _, f := range task.torrent.Files() {
		if f.Path() == path {
			f.Download()
			r := f.NewReader()
			r.SetResponsive()
			r.SetReadahead(2 * 1024 * 1024)
			return r, nil
		}
	}
	return nil, ErrFileNotFound
}

func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	task := m.tasks[id]
	if task != nil && task.torrent != nil {
		task.torrent.Drop()
	}
	delete(m.tasks, id)
}

func (m *Manager) BytesCompleted(id string) (completed int64, length int64, name string) {
	m.mu.Lock()
	task := m.tasks[id]
	m.mu.Unlock()
	if task == nil || task.torrent == nil {
		return 0, 0, taskName(task)
	}
	t := task.torrent
	if info := t.Info(); info != nil {
		name = info.Name
	} else {
		name = taskName(task)
	}
	return t.BytesCompleted(), t.Length(), name
}

func taskName(task *Task) string {
	if task == nil {
		return ""
	}
	if task.Name != "" {
		return task.Name
	}
	return ""
}
