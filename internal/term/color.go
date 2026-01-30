// Package term provides terminal styling and color output.
package term

import "fmt"

const (
	// Reset resets the terminal color.
	Reset = "\033[0m"
	// Bold makes the text bold.
	Bold = "\033[1m"
	// Dim makes the text dim.
	Dim = "\033[2m"
	// Red makes the text red.
	Red = "\033[31m"
	// Green makes the text green.
	Green = "\033[32m"
	// Yellow makes the text yellow.
	Yellow = "\033[33m"
	// Blue makes the text blue.
	Blue = "\033[34m"
	// Magenta makes the text magenta.
	Magenta = "\033[35m"
	// Cyan makes the text cyan.
	Cyan = "\033[36m"
	// Gray makes the text gray.
	Gray = "\033[37m"
)

// Styler handles color formatting with cached capability detection.
type Styler struct {
	enabled bool
}

// Global default styler.
var std = NewStyler()

// NewStyler creates a new Styler with auto-detected color support.
func NewStyler() *Styler {
	return &Styler{
		enabled: ShouldUseColor(),
	}
}

// Enabled returns true if colors are enabled for this styler.
func (s *Styler) Enabled() bool {
	return s.enabled
}

// Colorize wraps text in color code if colors are enabled.
func (s *Styler) Colorize(code, text string) string {
	if !s.enabled {
		return text
	}
	return code + text + Reset
}

// Helper methods on Styler

// Red formats text in red.
func (s *Styler) Red(format string, a ...any) string {
	return s.Colorize(Red, fmt.Sprintf(format, a...))
}

// Green formats text in green.
func (s *Styler) Green(format string, a ...any) string {
	return s.Colorize(Green, fmt.Sprintf(format, a...))
}

// Yellow formats text in yellow.
func (s *Styler) Yellow(format string, a ...any) string {
	return s.Colorize(Yellow, fmt.Sprintf(format, a...))
}

// Blue formats text in blue.
func (s *Styler) Blue(format string, a ...any) string {
	return s.Colorize(Blue, fmt.Sprintf(format, a...))
}

// Cyan formats text in cyan.
func (s *Styler) Cyan(format string, a ...any) string {
	return s.Colorize(Cyan, fmt.Sprintf(format, a...))
}

// Magenta formats text in magenta.
func (s *Styler) Magenta(format string, a ...any) string {
	return s.Colorize(Magenta, fmt.Sprintf(format, a...))
}

// Bold formats text in bold.
func (s *Styler) Bold(format string, a ...any) string {
	return s.Colorize(Bold, fmt.Sprintf(format, a...))
}

// Dim formats text in dim.
func (s *Styler) Dim(format string, a ...any) string {
	return s.Colorize(Dim, fmt.Sprintf(format, a...))
}

// Package-level convenience functions using the default styler

// RedString formats text in red using the default styler.
func RedString(format string, a ...any) string {
	return std.Red(format, a...)
}

// GreenString formats text in green using the default styler.
func GreenString(format string, a ...any) string {
	return std.Green(format, a...)
}

// YellowString formats text in yellow using the default styler.
func YellowString(format string, a ...any) string {
	return std.Yellow(format, a...)
}

// BlueString formats text in blue using the default styler.
func BlueString(format string, a ...any) string {
	return std.Blue(format, a...)
}

// CyanString formats text in cyan using the default styler.
func CyanString(format string, a ...any) string {
	return std.Cyan(format, a...)
}

// MagentaString formats text in magenta using the default styler.
func MagentaString(format string, a ...any) string {
	return std.Magenta(format, a...)
}

// BoldString formats text in bold using the default styler.
func BoldString(format string, a ...any) string {
	return std.Bold(format, a...)
}

// DimString formats text in dim using the default styler.
func DimString(format string, a ...any) string {
	return std.Dim(format, a...)
}

// Printf formats according to a format specifier and writes to standard output.
func Printf(format string, a ...any) (n int, err error) {
	return fmt.Printf(format, a...)
}

// Println formats using the default formats for its operands and writes to standard output.
func Println(a ...any) (n int, err error) {
	return fmt.Println(a...)
}
