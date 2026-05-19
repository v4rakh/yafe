package output

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	timeFormat = "2006-01-02 15:04:05"
	emptyValue = "-"
)

// Table renders data as aligned columns.
type Table struct {
	headers []string
	rows    [][]string
	widths  []int
}

// NewTable creates a new table with the given headers.
func NewTable(headers ...string) *Table {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	return &Table{
		headers: headers,
		widths:  widths,
	}
}

// AddRow adds a row to the table.
func (t *Table) AddRow(values ...string) {
	// Pad or truncate to match header count
	row := make([]string, len(t.headers))
	for i := range row {
		if i < len(values) {
			row[i] = values[i]
		} else {
			row[i] = emptyValue
		}
		if len(row[i]) > t.widths[i] {
			t.widths[i] = len(row[i])
		}
	}
	t.rows = append(t.rows, row)
}

// Render writes the table to the given writer.
func (t *Table) Render(w io.Writer) {
	// Print header
	t.printRow(w, t.headers)

	// Print rows
	for _, row := range t.rows {
		t.printRow(w, row)
	}
}

func (t *Table) printRow(w io.Writer, values []string) {
	for i, v := range values {
		if i > 0 {
			fmt.Fprint(w, "  ")
		}
		fmt.Fprintf(w, "%-*s", t.widths[i], v)
	}
	fmt.Fprintln(w)
}

// KeyValue holds a key-value pair for display.
type KeyValue struct {
	Key   string
	Value string
}

// PrintKeyValuesWithIndent prints key-value pairs, supporting indented sub-values.
func PrintKeyValuesWithIndent(w io.Writer, pairs []KeyValue, indent string) {
	// Find max key length
	maxLen := 0
	for _, p := range pairs {
		if len(p.Key) > maxLen {
			maxLen = len(p.Key)
		}
	}

	// Print pairs
	for _, p := range pairs {
		if strings.Contains(p.Value, "\n") {
			// Multi-line value
			fmt.Fprintf(w, "%s%-*s\n", indent, maxLen+1, p.Key+":")
			for _, line := range strings.Split(p.Value, "\n") {
				if line != "" {
					fmt.Fprintf(w, "%s  %s\n", indent, line)
				}
			}
		} else {
			fmt.Fprintf(w, "%s%-*s  %s\n", indent, maxLen+1, p.Key+":", p.Value)
		}
	}
}

// FormatTime formats a time pointer for display.
func FormatTime(t *time.Time) string {
	if t == nil {
		return emptyValue
	}
	return t.Format(timeFormat)
}

// FormatTimeValue formats a time value for display.
func FormatTimeValue(t time.Time) string {
	return t.Format(timeFormat)
}

// FormatString returns the string or empty placeholder if empty.
func FormatString(s string) string {
	if s == "" {
		return emptyValue
	}
	return s
}

// FormatInt formats an int pointer for display.
func FormatInt(i *int) string {
	if i == nil {
		return emptyValue
	}
	return strconv.Itoa(*i)
}

// TruncateString truncates a string to maxLen, adding "..." if truncated.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
