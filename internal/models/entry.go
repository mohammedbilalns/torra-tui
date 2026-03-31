package models

import "time"

type TorrentEntry struct {
	ID string
	Magnet string
	Name string
	DownloadDir string
	Completd bool
	CreatedAt time.Time
}

