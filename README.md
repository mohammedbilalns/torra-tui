# torra-tui

TUI torrent client.

## Requirements

- Go 1.22+(only for building)

## Install / Run
Copy the binary to any directory on your `PATH`:

```bash
cp ./bin/torra-tui /path/on/your/PATH/
```

Make sure the destination directory is on your `PATH`.

## Config

`~/.config/torra-tui/config.toml`:

```toml
download_dir = "/path/to/downloads"
max_parallel_downloads = 3
download_rate_limit_kbps = 0
upload_rate_limit_kbps = 0
```

## Keybindings

- `a` add magnet
- `r` resume
- `s` pause
- `d` delete (with confirmation)
- `c` change download directory
- `x` reset (delete DB + config + files, then exit)
- `q` quit
