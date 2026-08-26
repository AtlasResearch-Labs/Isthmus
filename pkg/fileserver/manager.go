package fileserver

import (
	"fmt"
	"strings"
)

func FormatBytes(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024.0)
	} else if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024.0*1024.0))
	}
	return fmt.Sprintf("%.2f GB", float64(bytes)/(1024.0*1024.0*1024.0))
}

func FormatSpeed(bytesPerSec float64) string {
	if bytesPerSec < 1024 {
		return fmt.Sprintf("%6.0f B/s", bytesPerSec)
	} else if bytesPerSec < 1024*1024 {
		return fmt.Sprintf("%6.1f KB/s", bytesPerSec/1024.0)
	}
	return fmt.Sprintf("%6.2f MB/s", bytesPerSec/(1024.0*1024.0))
}

func FormatETA(remainingBytes int64, speed float64) string {
	if speed <= 0 || remainingBytes <= 0 {
		return "--:--"
	}
	seconds := int(float64(remainingBytes) / speed)
	if seconds > 3600 {
		return fmt.Sprintf("%02dh:%02dm", seconds/3600, (seconds%3600)/60)
	}
	return fmt.Sprintf("%02d:%02d", seconds/60, seconds%60)
}

func RenderProgressBar(transferred, total int64, speed float64, barWidth int) string {
	if barWidth <= 0 {
		barWidth = 30
	}

	var percent float64
	if total > 0 {
		percent = float64(transferred) / float64(total) * 100.0
	}
	if percent > 100.0 {
		percent = 100.0
	}

	filled := int((percent / 100.0) * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("=", filled)
	if filled < barWidth {
		bar += ">" + strings.Repeat(" ", barWidth-filled-1)
	}

	remaining := total - transferred
	if remaining < 0 {
		remaining = 0
	}

	etaStr := FormatETA(remaining, speed)
	speedStr := FormatSpeed(speed)
	transStr := FormatBytes(transferred)
	totStr := FormatBytes(total)

	return fmt.Sprintf("[%s] %5.1f%% [ %s ] [ ETA %s ] ( %s / %s )",
		bar, percent, speedStr, etaStr, transStr, totStr)
}
