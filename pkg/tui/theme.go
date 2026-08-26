package tui

import (
	"fmt"
	"strings"
)

const (
	ColorReset      = "\033[0m"
	ColorBold       = "\033[1m"
	ColorDim        = "\033[2m"
	ColorUnderline  = "\033[4m"
	ColorInvert     = "\033[7m"

	// Foreground Colors
	ColorFgBlack   = "\033[30m"
	ColorFgWhite   = "\033[97m"
	ColorFgGray    = "\033[90m"
	ColorFgLightGray = "\033[37m"
	ColorFgBlue    = "\033[94m"
	ColorFgDarkBlue= "\033[34m"
	ColorFgCyan    = "\033[96m"

	// Background Colors
	ColorBgBlack   = "\033[40m"            // OLED Black
	ColorBgOLED    = "\033[48;2;0;0;0m"    // True OLED Black (24-bit)
	ColorBgWinBlue = "\033[48;2;0;0;128m"  // Retro Windows Title/Selection Blue
	ColorBgDarkBlue= "\033[44m"
	ColorBgGray    = "\033[48;2;40;40;40m"
	ColorBgWhite   = "\033[107m"
)

const (
	SymDir      = "[DIR ]"
	SymFile     = "[FILE]"
	SymCursor   = "->"
	SymEmpty    = "  "
	SymOK       = "[OK]"
	SymWarn     = "[WARN]"
	SymErr      = "[ERR]"
	SymNode     = "[*]"
	SymLAN      = "[LAN]"
	SymWAN      = "[WAN]"
	SymRelay    = "[RELAY]"

	// Retro Windows Box Drawing
	BoxTopLeft     = "+"
	BoxTopRight    = "+"
	BoxBottomLeft  = "+"
	BoxBottomRight = "+"
	BoxHorizontal  = "-"
	BoxVertical    = "|"
	BoxCross       = "+"
	BoxTeeLeft     = "+"
	BoxTeeRight    = "+"
)

func RetroTitleBar(title string, width int) string {
	if width <= 0 {
		width = 80
	}
	content := fmt.Sprintf(" [%s] ", title)
	padTotal := width - len(content)
	if padTotal < 0 {
		padTotal = 0
	}
	padLeft := padTotal / 2
	padRight := padTotal - padLeft

	return fmt.Sprintf("%s%s%s%s%s%s%s",
		ColorBgWinBlue, ColorFgWhite, ColorBold,
		strings.Repeat("=", padLeft),
		content,
		strings.Repeat("=", padRight),
		ColorReset,
	)
}

func RetroStatusBar(shortcuts string, width int) string {
	if width <= 0 {
		width = 80
	}
	content := fmt.Sprintf(" %s", shortcuts)
	pad := width - len(content)
	if pad < 0 {
		pad = 0
	}

	return fmt.Sprintf("%s%s%s%s%s%s",
		ColorBgWinBlue, ColorFgWhite,
		content,
		strings.Repeat(" ", pad),
		ColorReset,
		"\n",
	)
}

func RetroHorizontalDivider(width int) string {
	if width <= 0 {
		width = 80
	}
	return fmt.Sprintf("%s%s%s", ColorFgGray, strings.Repeat("-", width), ColorReset)
}
