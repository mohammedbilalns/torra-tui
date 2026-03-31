package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderView() string {
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
	if m.mode == modePromptPlay {
		content := "Video file detected.\n\nPress p to play now, d to download only."
		return m.fullscreen(frame.Render(top + "\n\n" + box.Render(content) + "\n\n" + status))
	}
	if m.mode == modeSelectVideo {
		var lines []string
		for i, vf := range m.videoFiles {
			cursor := " "
			if i == m.videoSelect {
				cursor = "›"
			}
			lines = append(lines, fmt.Sprintf("%s %s", cursor, trimTo(vf.Path, 60)))
		}
		content := "Select video file to play:\n\n" + strings.Join(lines, "\n") + "\n\nEnter to play, Esc to cancel."
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
