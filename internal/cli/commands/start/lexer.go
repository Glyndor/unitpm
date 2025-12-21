package start

import (
	"errors"
	"strings"
	"unicode"
)

// tokenize parses a command line string into arguments, handling quotes and escapes.
// It does NOT support shell features like globbing or env expansion.
func tokenize(input string) ([]string, error) {
	var args []string
	var current strings.Builder
	
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false
	
	for _, r := range input {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		
		if r == '\\' {
			if inSingleQuote {
				current.WriteRune(r)
			} else {
				escaped = true
			}
			continue
		}
		
		if inSingleQuote {
			if r == '\'' {
				inSingleQuote = false
			} else {
				current.WriteRune(r)
			}
			continue
		}
		
		if inDoubleQuote {
			if r == '"' {
				inDoubleQuote = false
			} else {
				current.WriteRune(r)
			}
			continue
		}
		
		if r == '\'' {
			inSingleQuote = true
			continue
		}
		
		if r == '"' {
			inDoubleQuote = true
			continue
		}
		
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
			continue
		}
		
		current.WriteRune(r)
	}
	
	if escaped {
		return nil, errors.New("unexpected end of input: trailing backslash")
	}
	
	if inSingleQuote || inDoubleQuote {
		return nil, errors.New("unexpected end of input: unclosed quote")
	}
	
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	
	return args, nil
}
