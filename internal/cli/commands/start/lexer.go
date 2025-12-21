package start

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// tokenize parses a command line string into arguments, handling quotes and escapes.
// It does NOT support shell features like globbing or env expansion.
func tokenize(input string) ([]string, error) {
	var args []string
	var current strings.Builder

	// State machine states
	const (
		stateNormal = iota
		stateSingle
		stateDouble
	)
	state := stateNormal

	runes := []rune(input)

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		switch state {
		case stateNormal:
			if unicode.IsSpace(r) {
				if current.Len() > 0 {
					args = append(args, current.String())
					current.Reset()
				}
			} else if r == '\'' {
				state = stateSingle
			} else if r == '"' {
				state = stateDouble
			} else {
				// Treat everything else literally, including backslash, pipe, etc.
				current.WriteRune(r)
			}

		case stateSingle:
			if r == '\'' {
				state = stateNormal
			} else {
				// Inside single quotes, everything is literal, including backslash
				current.WriteRune(r)
			}

		case stateDouble:
			if r == '"' {
				state = stateNormal
			} else if r == '\\' {
				// Handle escape sequence
				if i+1 >= len(runes) {
					return nil, errors.New("invalid escape sequence: trailing backslash")
				}
				next := runes[i+1]
				// Only allow escaping " and \ inside double quotes
				switch next {
				case '"', '\\':
					current.WriteRune(next)
					i++ // skip next
				default:
					return nil, fmt.Errorf("invalid escape sequence: \\%c", next)
				}
			} else {
				current.WriteRune(r)
			}
		}
	}

	if state != stateNormal {
		return nil, errors.New("unclosed quote")
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args, nil
}
