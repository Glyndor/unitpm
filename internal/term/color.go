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

var enabled = false

func init() {
	enabled = ShouldUseColor()
}

// Colorize wraps text in color code if colors are enabled
func Colorize(code, text string) string {
	if !enabled {
		return text
	}
	return code + text + Reset
}

// Helper functions for common colors

func RedString(format string, a ...interface{}) string {
	return Colorize(Red, fmt.Sprintf(format, a...))
}

func GreenString(format string, a ...interface{}) string {
	return Colorize(Green, fmt.Sprintf(format, a...))
}

func YellowString(format string, a ...interface{}) string {
	return Colorize(Yellow, fmt.Sprintf(format, a...))
}

func BlueString(format string, a ...interface{}) string {
	return Colorize(Blue, fmt.Sprintf(format, a...))
}

func CyanString(format string, a ...interface{}) string {
	return Colorize(Cyan, fmt.Sprintf(format, a...))
}

func BoldString(format string, a ...interface{}) string {
	return Colorize(Bold, fmt.Sprintf(format, a...))
}
