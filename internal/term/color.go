package term

import "fmt"

const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	Gray    = "\033[37m"
)

// Styler handles color formatting with cached capability detection
type Styler struct {
	enabled bool
}

// Global default styler
var std = NewStyler()

// NewStyler creates a new Styler with auto-detected color support
func NewStyler() *Styler {
	return &Styler{
		enabled: ShouldUseColor(),
	}
}

// Enabled returns true if colors are enabled for this styler
func (s *Styler) Enabled() bool {
	return s.enabled
}

// Colorize wraps text in color code if colors are enabled
func (s *Styler) Colorize(code, text string) string {
	if !s.enabled {
		return text
	}
	return code + text + Reset
}

// Helper methods on Styler

func (s *Styler) Red(format string, a ...interface{}) string {
	return s.Colorize(Red, fmt.Sprintf(format, a...))
}

func (s *Styler) Green(format string, a ...interface{}) string {
	return s.Colorize(Green, fmt.Sprintf(format, a...))
}

func (s *Styler) Yellow(format string, a ...interface{}) string {
	return s.Colorize(Yellow, fmt.Sprintf(format, a...))
}

func (s *Styler) Blue(format string, a ...interface{}) string {
	return s.Colorize(Blue, fmt.Sprintf(format, a...))
}

func (s *Styler) Cyan(format string, a ...interface{}) string {
	return s.Colorize(Cyan, fmt.Sprintf(format, a...))
}

func (s *Styler) Magenta(format string, a ...interface{}) string {
	return s.Colorize(Magenta, fmt.Sprintf(format, a...))
}

func (s *Styler) Bold(format string, a ...interface{}) string {
	return s.Colorize(Bold, fmt.Sprintf(format, a...))
}

func (s *Styler) Dim(format string, a ...interface{}) string {
	return s.Colorize(Dim, fmt.Sprintf(format, a...))
}

// Package-level convenience functions using the default styler

func RedString(format string, a ...interface{}) string {
	return std.Red(format, a...)
}

func GreenString(format string, a ...interface{}) string {
	return std.Green(format, a...)
}

func YellowString(format string, a ...interface{}) string {
	return std.Yellow(format, a...)
}

func BlueString(format string, a ...interface{}) string {
	return std.Blue(format, a...)
}

func CyanString(format string, a ...interface{}) string {
	return std.Cyan(format, a...)
}

func MagentaString(format string, a ...interface{}) string {
	return std.Magenta(format, a...)
}

func BoldString(format string, a ...interface{}) string {
	return std.Bold(format, a...)
}

func DimString(format string, a ...interface{}) string {
	return std.Dim(format, a...)
}
