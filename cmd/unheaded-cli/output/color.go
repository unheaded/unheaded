// Package output provides color support for THE GAUNTLETS CLI.
package output

import (
	"os"
	"strings"
)

// ANSI color codes.
const (
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
	ColorDim    = "\033[2m"
	ColorItalic = "\033[3m"

	ColorBlack   = "\033[30m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"

	ColorBrightBlack   = "\033[90m"
	ColorBrightRed     = "\033[91m"
	ColorBrightGreen   = "\033[92m"
	ColorBrightYellow  = "\033[93m"
	ColorBrightBlue    = "\033[94m"
	ColorBrightMagenta = "\033[95m"
	ColorBrightCyan    = "\033[96m"
	ColorBrightWhite   = "\033[97m"

	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"
)

// IsTerminal checks if stdout is a terminal.
func IsTerminal() bool {
	// Check for explicit NO_COLOR environment variable
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}

	// Check for explicit FORCE_COLOR environment variable
	if _, ok := os.LookupEnv("FORCE_COLOR"); ok {
		return true
	}

	// Check TERM environment variable
	term := os.Getenv("TERM")
	if term == "dumb" {
		return false
	}

	// Check if stdout is a file (pipe or redirect)
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	// Check if it's a character device (terminal)
	return fileInfo.Mode()&os.ModeCharDevice != 0
}

// ColorizeStatus returns a status string with appropriate color.
func ColorizeStatus(status string, enabled bool) string {
	if !enabled {
		return status
	}
	return StatusOutput{Status: status}.Colorize(enabled)
}

// Bold wraps text in bold formatting.
func Bold(s string) string {
	return ColorBold + s + ColorReset
}

// Dim wraps text in dim formatting.
func Dim(s string) string {
	return ColorDim + s + ColorReset
}

// Red wraps text in red color.
func Red(s string) string {
	return ColorRed + s + ColorReset
}

// Green wraps text in green color.
func Green(s string) string {
	return ColorGreen + s + ColorReset
}

// Yellow wraps text in yellow color.
func Yellow(s string) string {
	return ColorYellow + s + ColorReset
}

// Blue wraps text in blue color.
func Blue(s string) string {
	return ColorBlue + s + ColorReset
}

// Cyan wraps text in cyan color.
func Cyan(s string) string {
	return ColorCyan + s + ColorReset
}

// Magenta wraps text in magenta color.
func Magenta(s string) string {
	return ColorMagenta + s + ColorReset
}

// HighlightMatch highlights a search match in a string.
func HighlightMatch(s, match string, enabled bool) string {
	if !enabled || match == "" {
		return s
	}
	highlighted := ColorYellow + ColorBold + match + ColorReset
	return strings.ReplaceAll(s, match, highlighted)
}

// ColorTheme defines a color theme for the CLI.
type ColorTheme struct {
	Primary   string
	Secondary string
	Success   string
	Warning   string
	Error     string
	Info      string
	Muted     string
}

// DefaultTheme returns the default color theme.
func DefaultTheme() ColorTheme {
	return ColorTheme{
		Primary:   ColorCyan,
		Secondary: ColorMagenta,
		Success:   ColorGreen,
		Warning:   ColorYellow,
		Error:     ColorRed,
		Info:      ColorBlue,
		Muted:     ColorDim,
	}
}

// Apply applies a theme color to text.
func (t ColorTheme) Apply(style, text string) string {
	var color string
	switch style {
	case "primary":
		color = t.Primary
	case "secondary":
		color = t.Secondary
	case "success":
		color = t.Success
	case "warning":
		color = t.Warning
	case "error":
		color = t.Error
	case "info":
		color = t.Info
	case "muted":
		color = t.Muted
	default:
		return text
	}
	return color + text + ColorReset
}

// Banner returns a styled banner for the CLI.
func Banner(text string, color bool) string {
	lines := strings.Split(text, "\n")
	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}

	border := strings.Repeat("=", maxLen+4)

	var sb strings.Builder
	if color {
		sb.WriteString(ColorCyan + ColorBold)
	}
	sb.WriteString(border + "\n")
	for _, line := range lines {
		padding := maxLen - len(line)
		sb.WriteString("| " + line + strings.Repeat(" ", padding) + " |\n")
	}
	sb.WriteString(border)
	if color {
		sb.WriteString(ColorReset)
	}
	sb.WriteString("\n")

	return sb.String()
}

// GradientText applies a horizontal gradient effect to text (using color palette).
func GradientText(text string, enabled bool) string {
	if !enabled {
		return text
	}

	colors := []string{ColorBlue, ColorCyan, ColorGreen, ColorYellow}
	runes := []rune(text)
	result := strings.Builder{}

	for i, r := range runes {
		colorIdx := (i * len(colors)) / len(runes)
		if colorIdx >= len(colors) {
			colorIdx = len(colors) - 1
		}
		result.WriteString(colors[colorIdx])
		result.WriteRune(r)
	}
	result.WriteString(ColorReset)

	return result.String()
}

// Icon returns an icon character (Unicode).
type Icon string

const (
	IconSuccess     Icon = "\u2714" // check mark
	IconError       Icon = "\u2718" // cross mark
	IconWarning     Icon = "\u26A0" // warning
	IconInfo        Icon = "\u2139" // info
	IconArrowRight  Icon = "\u2192" // right arrow
	IconArrowLeft   Icon = "\u2190" // left arrow
	IconArrowUp     Icon = "\u2191" // up arrow
	IconArrowDown   Icon = "\u2193" // down arrow
	IconBullet      Icon = "\u2022" // bullet
	IconStar        Icon = "\u2605" // star
	IconCircle      Icon = "\u25CF" // filled circle
	IconSquare      Icon = "\u25A0" // filled square
	IconTriangle    Icon = "\u25B6" // right triangle
	IconDiamond     Icon = "\u25C6" // diamond
	IconContainer   Icon = "\u2395" // container (like box)
	IconNetwork     Icon = "\u2261" // network (triple bar)
	IconLock        Icon = "\u26BF" // lock
	IconUnlock      Icon = "\u26BF" // unlock (same for simplicity)
	IconClock       Icon = "\u23F0" // clock
	IconGear        Icon = "\u2699" // gear
	IconFolder      Icon = "\u1F4C1" // folder
	IconFile        Icon = "\u1F4C4" // file
)

// String returns the icon as a string.
func (i Icon) String() string {
	return string(i)
}

// WithColor returns the icon with the specified color.
func (i Icon) WithColor(color string) string {
	return color + string(i) + ColorReset
}

// StatusIcon returns an appropriate icon for a status.
func StatusIcon(status string, enabled bool) string {
	if !enabled {
		return ""
	}

	switch strings.ToLower(status) {
	case "running", "healthy", "active", "success", "ok":
		return Green(string(IconSuccess))
	case "stopped", "error", "failed", "unhealthy":
		return Red(string(IconError))
	case "pending", "starting", "stopping", "warning":
		return Yellow(string(IconWarning))
	default:
		return Cyan(string(IconInfo))
	}
}
