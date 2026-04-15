// export_test.go exposes unexported functions for white-box testing.
// Only compiled during `go test`.
package list

var (
	FormatUptime    = formatUptime
	FormatBytes     = formatBytes
	ShortIDLen      = shortIDLen
	FilterProcesses = filterProcesses
)
