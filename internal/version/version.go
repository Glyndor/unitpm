// Package version provides build information and versioning for Lynx.
package version

// These variables are set at build time using -ldflags.
var (
	// Version is the current version of Lynx.
	Version = "0.4.6"

	// Commit is the git commit hash of the build.
	Commit = "none"

	// BuildDate is the date when the binary was built.
	BuildDate = "unknown"

	// ProtocolVersion is the current IPC protocol version.
	ProtocolVersion = 1
)

// Info holds version information for serialization.
type Info struct {
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	BuildDate       string `json:"build_date"`
	ProtocolVersion int    `json:"protocol_version"`
}

// Get returns the current version information.
func Get() Info {
	return Info{
		Version:         Version,
		Commit:          Commit,
		BuildDate:       BuildDate,
		ProtocolVersion: ProtocolVersion,
	}
}
