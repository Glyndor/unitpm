package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type Table struct {
	Headers []string
	Rows    [][]string
}

func NewTable(headers []string) *Table {
	return &Table{
		Headers: headers,
		Rows:    [][]string{},
	}
}

func (t *Table) AddRow(row []string) {
	t.Rows = append(t.Rows, row)
}

func (t *Table) Render() {
	// Calculate column widths
	widths := make([]int, len(t.Headers))
	for i, h := range t.Headers {
		widths[i] = utf8.RuneCountInString(h)
	}

	for _, row := range t.Rows {
		for i, cell := range row {
			// Strip ANSI codes for length calculation
			len := utf8.RuneCountInString(stripAnsi(cell))
			if len > widths[i] {
				widths[i] = len
			}
		}
	}

	// Print Top Border
	// ┌───┬───┐
	fmt.Print("┌")
	for i, w := range widths {
		fmt.Print(strings.Repeat("─", w+2)) // +2 for padding
		if i < len(widths)-1 {
			fmt.Print("┬")
		}
	}
	fmt.Println("┐")

	// Print Headers
	fmt.Print("│")
	for i, h := range t.Headers {
		fmt.Printf(" %-*s │", widths[i], h)
	}
	fmt.Println()

	// Print Header Separator
	// ├───┼───┤
	fmt.Print("├")
	for i, w := range widths {
		fmt.Print(strings.Repeat("─", w+2))
		if i < len(widths)-1 {
			fmt.Print("┼")
		}
	}
	fmt.Println("┤")

	// Print Rows
	for _, row := range t.Rows {
		fmt.Print("│")
		for i, cell := range row {
			// We need to pad manually because Printf padding counts ANSI codes as characters
			// So we calculate the visible length
			visibleLen := utf8.RuneCountInString(stripAnsi(cell))
			padding := widths[i] - visibleLen
			fmt.Printf(" %s%s │", cell, strings.Repeat(" ", padding))
		}
		fmt.Println()
	}

	// Print Bottom Border
	// └───┴───┘
	fmt.Print("└")
	for i, w := range widths {
		fmt.Print(strings.Repeat("─", w+2))
		if i < len(widths)-1 {
			fmt.Print("┴")
		}
	}
	fmt.Println("┘")
}

// Simple ANSI stripper for length calculation
func stripAnsi(str string) string {
	var ret strings.Builder
	inSeq := false
	for _, r := range str {
		if r == '\033' {
			inSeq = true
			continue
		}
		if inSeq {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inSeq = false
			}
			continue
		}
		ret.WriteRune(r)
	}
	return ret.String()
}
