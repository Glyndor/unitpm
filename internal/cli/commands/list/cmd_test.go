package list_test

import (
	"reflect"
	"testing"

	"github.com/Jaro-c/Lynx/internal/cli/commands/list"
)

// We can test parseSortSpec since it's exported logic, but we need to export it in the list package
// or use export_test.go. However, since the function is private, we should create a test file in the same package
// or rely on a helper.
// For simplicity, let's test the unexported function by being in package list.

func TestParseSortSpec(t *testing.T) {
	tests := []struct {
		input   string
		want    []list.SortField
		wantErr bool
	}{
		{
			input: "",
			want:  nil,
		},
		{
			input: "name",
			want: []list.SortField{
				{Field: "name", Asc: true},
			},
		},
		{
			input: "name:desc",
			want: []list.SortField{
				{Field: "name", Asc: false},
			},
		},
		{
			input: "namespace:asc, name:desc",
			want: []list.SortField{
				{Field: "namespace", Asc: true},
				{Field: "name", Asc: false},
			},
		},
		{
			input:   "invalid",
			wantErr: true,
		},
		{
			input:   "name:invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := list.ParseSortSpec(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseSortSpec(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseSortSpec(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
