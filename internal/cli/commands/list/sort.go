package list

import (
	"strings"

	"github.com/Jaro-c/Lynx/internal/cli/errs"
)

// SortField represents a field to sort by.
type SortField struct {
	Field string
	Asc   bool
}

// ParseSortSpec parses a sort specification string.
func ParseSortSpec(spec string) ([]SortField, error) {
	if spec == "" {
		return nil, nil
	}

	parts := strings.Split(spec, ",")
	fields := make([]SortField, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		field := part
		asc := true
		if idx := strings.Index(part, ":"); idx != -1 {
			field = strings.TrimSpace(part[:idx])
			dir := strings.ToLower(strings.TrimSpace(part[idx+1:]))
			if dir == "desc" {
				asc = false
			} else if dir != "" && dir != "asc" {
				return nil, &errs.UsageError{Message: "invalid sort direction: " + dir}
			}
		}
		switch field {
		case "namespace", "name", "createdAt", "id":
		default:
			return nil, &errs.UsageError{Message: "invalid sort field: " + field}
		}
		fields = append(fields, SortField{Field: field, Asc: asc})
	}
	return fields, nil
}
