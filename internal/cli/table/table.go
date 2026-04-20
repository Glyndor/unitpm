// Package table renders box-drawing tables sized to the terminal width.
//
// Two shapes are supported:
//
//   - Table: a classic column table (headers + rows), auto-wrapping long
//     cells and shrinking the widest column when the total exceeds the
//     terminal width.
//   - KV (key/value): a compact two-column layout with an optional section
//     title, used by commands like `show` to render AppSpec sections.
package table

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	xterm "golang.org/x/term"

	"github.com/Jaro-c/Lynx/internal/cli/format"
	"github.com/Jaro-c/Lynx/internal/term"
)

// Table represents a printable table.
type Table struct {
	headers      []string
	rows         [][]string
	maxColWidths []int
}

// New creates a new Table with the given headers.
func New(headers []string) *Table {
	return &Table{headers: headers}
}

// AddRow adds a row of data to the table.
func (t *Table) AddRow(row []string) { t.rows = append(t.rows, row) }

// SetMaxColWidths configures per-column maximum widths. Columns wider than
// their max are wrapped. Length must match the header count, otherwise the
// value is silently ignored.
func (t *Table) SetMaxColWidths(widths []int) {
	if len(widths) == len(t.headers) {
		t.maxColWidths = widths
	}
}

// Render prints the table to stdout, sized to the current terminal width.
func (t *Table) Render() {
	width, _, err := xterm.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		width = 120
	}
	widths := t.calculateWidths(width)
	t.printBorder("┌", "┬", "┐", widths)
	t.printRow(t.headers, widths)
	t.printBorder("├", "┼", "┤", widths)
	for _, row := range t.rows {
		t.printRow(row, widths)
	}
	t.printBorder("└", "┴", "┘", widths)
}

func (t *Table) calculateWidths(termWidth int) []int {
	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = utf8.RuneCountInString(format.StripAnsi(h))
	}
	for _, row := range t.rows {
		for i, cell := range row {
			l := utf8.RuneCountInString(format.StripAnsi(cell))
			if l > widths[i] {
				widths[i] = l
			}
		}
	}
	if len(t.maxColWidths) == len(widths) {
		for i, maxW := range t.maxColWidths {
			if widths[i] > maxW {
				widths[i] = maxW
			}
		}
	}

	const minColWidth = 3
	for {
		totalWidth := 1
		for _, w := range widths {
			totalWidth += w + 3
		}
		if totalWidth <= termWidth {
			break
		}
		widestIdx := -1
		for i, w := range widths {
			if w <= minColWidth {
				continue
			}
			if widestIdx == -1 || w > widths[widestIdx] {
				widestIdx = i
			}
		}
		if widestIdx == -1 {
			break
		}
		widths[widestIdx]--
	}
	return widths
}

func (t *Table) printBorder(left, mid, right string, widths []int) {
	fmt.Print(term.DimString("%s", left))
	for i, w := range widths {
		fmt.Print(term.DimString("%s", strings.Repeat("─", w+2)))
		if i < len(widths)-1 {
			fmt.Print(term.DimString("%s", mid))
		}
	}
	fmt.Println(term.DimString("%s", right))
}

func (t *Table) printRow(row []string, widths []int) {
	cellLines := make([][]string, len(row))
	maxLines := 1
	for i, cell := range row {
		lines := wrapText(cell, widths[i])
		cellLines[i] = lines
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
	}
	for lineIdx := 0; lineIdx < maxLines; lineIdx++ {
		fmt.Print(term.DimString("│"))
		for i := range row {
			var cellContent string
			if lineIdx < len(cellLines[i]) {
				cellContent = cellLines[i][lineIdx]
			}
			visibleLen := utf8.RuneCountInString(format.StripAnsi(cellContent))
			padding := widths[i] - visibleLen
			if padding < 0 {
				padding = 0
			}
			fmt.Printf(" %s%s ", cellContent, strings.Repeat(" ", padding))
			fmt.Print(term.DimString("│"))
		}
		fmt.Println()
	}
}

// wrapText wraps a string to the given width. ANSI codes are assumed not
// to span break boundaries.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	if utf8.RuneCountInString(format.StripAnsi(text)) <= width {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	var lines []string
	currentLine := ""
	currentLen := 0
	for _, word := range words {
		wordLen := utf8.RuneCountInString(format.StripAnsi(word))
		if wordLen > width {
			if currentLen > 0 {
				lines = append(lines, currentLine)
				currentLine, currentLen = "", 0
			}
			lines = append(lines, splitLongWord(word, width)...)
			continue
		}
		if currentLen+wordLen+1 > width && currentLen > 0 {
			lines = append(lines, currentLine)
			currentLine, currentLen = word, wordLen
		} else if currentLen > 0 {
			currentLine += " " + word
			currentLen += 1 + wordLen
		} else {
			currentLine, currentLen = word, wordLen
		}
	}
	if currentLen > 0 {
		lines = append(lines, currentLine)
	}
	return lines
}

func splitLongWord(word string, width int) []string {
	var parts []string
	for len(word) > 0 {
		take := width
		if utf8.RuneCountInString(word) < take {
			take = utf8.RuneCountInString(word)
		}
		byteIdx, count := 0, 0
		for i := range word {
			if count == take {
				byteIdx = i
				break
			}
			count++
		}
		if byteIdx == 0 {
			byteIdx = len(word)
		}
		parts = append(parts, word[:byteIdx])
		word = word[byteIdx:]
	}
	return parts
}

// KV renders a titled key/value table. Rows with an empty value are
// omitted so callers can pass optional fields unconditionally.
//
// Example:
//
//	table.KV("Process", [][2]string{
//	    {"state", "running"},
//	    {"pid",   "1234"},
//	})
type KVRow [2]string

// KV prints a 2-column table with the given section title printed above.
// Empty values are dropped before rendering so the caller can supply
// optional fields unconditionally.
func KV(title string, rows []KVRow) {
	filtered := rows[:0]
	for _, r := range rows {
		if r[1] == "" {
			continue
		}
		filtered = append(filtered, r)
	}
	if len(filtered) == 0 {
		return
	}
	if title != "" {
		fmt.Println(term.BoldString("%s", title))
	}
	t := New([]string{term.CyanString("%s", term.BoldString("field")), term.CyanString("%s", term.BoldString("value"))})
	for _, r := range filtered {
		t.AddRow([]string{r[0], r[1]})
	}
	t.Render()
}
