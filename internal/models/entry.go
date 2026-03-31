package models

import "time"

type TorrentEntry struct {
	ID string
	Magnet string
	Name string
	DownloadDir string
	State string
	Completed bool
	BytesCompleted int64
	Length int64
	CreatedAt time.Time
}
