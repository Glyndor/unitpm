// export_test.go exposes unexported functions for white-box testing.
// Only compiled during `go test`.
package list

import "github.com/Jaro-c/Lynx/internal/cli/format"

var (
	FormatUptime    = format.Uptime
	FormatBytes     = format.Bytes
	ShortIDLen      = shortIDLen
	FilterProcesses = filterProcesses
)
