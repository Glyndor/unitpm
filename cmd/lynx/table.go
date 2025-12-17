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
	widths := t.calculateWidths()

	// Print Top Border
	t.printBorder("┌", "┬", "┐", widths)

	// Print Headers
	t.printRow(t.Headers, widths)

	// Print Header Separator
	t.printBorder("├", "┼", "┤", widths)

	// Print Rows
	for _, row := range t.Rows {
		t.printRow(row, widths)
	}

	// Print Bottom Border
	t.printBorder("└", "┴", "┘", widths)
}

func (t *Table) calculateWidths() []int {
	widths := make([]int, len(t.Headers))
	for i, h := range t.Headers {
		widths[i] = utf8.RuneCountInString(h)
	}

	for _, row := range t.Rows {
		for i, cell := range row {
			// Strip ANSI codes for length calculation
			l := utf8.RuneCountInString(stripAnsi(cell))
			if l > widths[i] {
				widths[i] = l
			}
		}
	}
	return widths
}

func (t *Table) printBorder(left, mid, right string, widths []int) {
	fmt.Print(left)
	for i, w := range widths {
		fmt.Print(strings.Repeat("─", w+2)) // +2 for padding
		if i < len(widths)-1 {
			fmt.Print(mid)
		}
	}
	fmt.Println(right)
}

func (t *Table) printRow(row []string, widths []int) {
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

// Simple ANSI stripper for length calculation.
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
