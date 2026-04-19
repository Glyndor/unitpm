package updater

import "testing"

// FuzzParseVersion feeds arbitrary strings to parseVersion to ensure the
// X.Y.Z splitter never panics on adversarial input (huge strings, NULs,
// mixed UTF-8, unbounded numeric segments). Any panic here is a bug
// because release tag names are user-visible and could be crafted.
func FuzzParseVersion(f *testing.F) {
	seeds := []string{
		"",
		"0",
		"0.0",
		"0.0.0",
		"1.2.3",
		"1.2.3.4",
		"v1.2.3",
		"abc",
		"1.a.3",
		"-1.-2.-3",
		"9999999999.9999999999.9999999999",
		"\x00.\x00.\x00",
		"1..2",
		"....",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(_ *testing.T, v string) {
		_ = parseVersion(v)
	})
}

// FuzzIsNewer feeds arbitrary pairs of strings to isNewer so semver
// comparison cannot be made to panic on adversarial input.
func FuzzIsNewer(f *testing.F) {
	pairs := [][2]string{
		{"", ""},
		{"1.0.0", "0.9.9"},
		{"abc", "1.2.3"},
		{"\x00", "\x00"},
		{"1.2", "1.2.3"},
		{"9999999999.0.0", "0.0.0"},
	}
	for _, p := range pairs {
		f.Add(p[0], p[1])
	}

	f.Fuzz(func(_ *testing.T, a, b string) {
		_ = isNewer(a, b)
	})
}
