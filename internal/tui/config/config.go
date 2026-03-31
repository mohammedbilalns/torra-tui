package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	DownloadDir           string `toml:"download_dir"`
	MaxParallelDownloads  int    `toml:"max_parallel_downloads"`
	DownloadRateLimitKbps int    `toml:"download_rate_limit_kbps"`
	UploadRateLimitKbps   int    `toml:"upload_rate_limit_kbps"`
}

func Default() Config {
	return Config{
		DownloadDir:           "",
		MaxParallelDownloads:  0,
		DownloadRateLimitKbps: 0,
		UploadRateLimitKbps:   0,
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
