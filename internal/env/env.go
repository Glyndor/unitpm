// Package env parses .env files into key/value maps with quoting and escape handling.
package env

import (
	"bufio"
	"os"
	"strings"
)

// ParseFile reads an env file and returns a map of key-value pairs.
// It handles comments, quoted values, and inline comments.
func ParseFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	env := make(map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		k, v, ok := parseLine(line)
		if ok {
			env[k] = v
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return env, nil
}

func parseLine(line string) (string, string, bool) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", false
	}

	val := parseValue(strings.TrimSpace(parts[1]))
	return key, val, true
}

func parseValue(val string) string {
	if val == "" {
		return ""
	}

	first := val[0]

	// Handle Double Quotes
	if first == '"' {
		return parseDoubleQuoted(val)
	}

	// Handle Single Quotes
	if first == '\'' {
		return parseSingleQuoted(val)
	}

	// Handle Unquoted
	return parseUnquoted(val)
}

func parseDoubleQuoted(val string) string {
	// Find closing quote properly (ignoring escaped ones)
	for i := 1; i < len(val); i++ {
		if val[i] == '"' && val[i-1] != '\\' {
			// Found closing quote
			// Return content, unescaping \" to "
			content := val[1:i]
			return strings.ReplaceAll(content, `\"`, `"`)
		}
	}
	// If no closing quote found, return as is (broken quote)
	return val
}

func parseSingleQuoted(val string) string {
	for i := 1; i < len(val); i++ {
		if val[i] == '\'' {
			return val[1:i]
		}
	}
	return val
}

func parseUnquoted(val string) string {
	// Strip comments starting with # preceded by space or tab
	for i := 0; i < len(val); i++ {
		if val[i] == '#' {
			// Check if preceded by whitespace
			if i > 0 && (val[i-1] == ' ' || val[i-1] == '\t') {
				return strings.TrimSpace(val[:i])
			}
		}
	}
	return strings.TrimSpace(val)
}
