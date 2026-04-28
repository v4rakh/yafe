package output

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewTable(t *testing.T) {
	t.Run("creates table with headers", func(t *testing.T) {
		table := NewTable("ID", "Name", "Status")

		assert.Equal(t, []string{"ID", "Name", "Status"}, table.headers)
		assert.Len(t, table.widths, 3)
	})
}

func TestTable_AddRow(t *testing.T) {
	t.Run("adds row", func(t *testing.T) {
		table := NewTable("ID", "Name")
		table.AddRow("1", "test")

		assert.Len(t, table.rows, 1)
		assert.Equal(t, []string{"1", "test"}, table.rows[0])
	})

	t.Run("pads missing values with placeholder", func(t *testing.T) {
		table := NewTable("ID", "Name", "Status")
		table.AddRow("1")

		assert.Equal(t, []string{"1", "-", "-"}, table.rows[0])
	})

	t.Run("updates column widths", func(t *testing.T) {
		table := NewTable("ID", "N")
		table.AddRow("123456", "longer-name")

		assert.Equal(t, 6, table.widths[0])  // "123456" is longer than "ID"
		assert.Equal(t, 11, table.widths[1]) // "longer-name" is longer than "N"
	})
}

func TestTable_Render(t *testing.T) {
	t.Run("renders table with headers and rows", func(t *testing.T) {
		table := NewTable("ID", "Name")
		table.AddRow("1", "alpha")
		table.AddRow("2", "beta")

		var buf bytes.Buffer
		table.Render(&buf)

		output := buf.String()
		assert.Contains(t, output, "ID")
		assert.Contains(t, output, "Name")
		assert.Contains(t, output, "alpha")
		assert.Contains(t, output, "beta")
	})

	t.Run("renders empty table with only headers", func(t *testing.T) {
		table := NewTable("ID", "Name")

		var buf bytes.Buffer
		table.Render(&buf)

		output := buf.String()
		assert.Contains(t, output, "ID")
		assert.Contains(t, output, "Name")
	})

	t.Run("aligns columns", func(t *testing.T) {
		table := NewTable("ID", "Name")
		table.AddRow("1", "short")
		table.AddRow("100", "longer-name")

		var buf bytes.Buffer
		table.Render(&buf)

		// Output should have proper alignment
		output := buf.String()
		assert.NotEmpty(t, output)
	})
}

func TestPrintKeyValuesWithIndent(t *testing.T) {
	t.Run("prints key-value pairs", func(t *testing.T) {
		pairs := []KeyValue{
			{Key: "Name", Value: "test"},
			{Key: "Status", Value: "running"},
		}

		var buf bytes.Buffer
		PrintKeyValuesWithIndent(&buf, pairs, "")

		output := buf.String()
		assert.Contains(t, output, "Name:")
		assert.Contains(t, output, "test")
		assert.Contains(t, output, "Status:")
		assert.Contains(t, output, "running")
	})

	t.Run("handles multi-line values", func(t *testing.T) {
		pairs := []KeyValue{
			{Key: "Output", Value: "line1\nline2\nline3"},
		}

		var buf bytes.Buffer
		PrintKeyValuesWithIndent(&buf, pairs, "")

		output := buf.String()
		assert.Contains(t, output, "Output:")
		assert.Contains(t, output, "line1")
		assert.Contains(t, output, "line2")
		assert.Contains(t, output, "line3")
	})

	t.Run("applies indent", func(t *testing.T) {
		pairs := []KeyValue{
			{Key: "Key", Value: "value"},
		}

		var buf bytes.Buffer
		PrintKeyValuesWithIndent(&buf, pairs, "  ")

		output := buf.String()
		assert.Contains(t, output, "  Key:")
	})
}

func TestFormatTime(t *testing.T) {
	t.Run("formats non-nil time", func(t *testing.T) {
		tm := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

		result := FormatTime(&tm)

		assert.Equal(t, "2024-01-15 10:30:45", result)
	})

	t.Run("returns placeholder for nil time", func(t *testing.T) {
		result := FormatTime(nil)

		assert.Equal(t, "-", result)
	})
}

func TestFormatTimeValue(t *testing.T) {
	t.Run("formats time value", func(t *testing.T) {
		tm := time.Date(2024, 6, 20, 14, 45, 30, 0, time.UTC)

		result := FormatTimeValue(tm)

		assert.Equal(t, "2024-06-20 14:45:30", result)
	})
}

func TestFormatString(t *testing.T) {
	t.Run("returns string when non-empty", func(t *testing.T) {
		result := FormatString("hello")

		assert.Equal(t, "hello", result)
	})

	t.Run("returns placeholder for empty string", func(t *testing.T) {
		result := FormatString("")

		assert.Equal(t, "-", result)
	})
}

func TestFormatInt(t *testing.T) {
	t.Run("formats non-nil int", func(t *testing.T) {
		val := 42

		result := FormatInt(&val)

		assert.Equal(t, "42", result)
	})

	t.Run("returns placeholder for nil", func(t *testing.T) {
		result := FormatInt(nil)

		assert.Equal(t, "-", result)
	})

	t.Run("formats zero", func(t *testing.T) {
		val := 0

		result := FormatInt(&val)

		assert.Equal(t, "0", result)
	})

	t.Run("formats negative", func(t *testing.T) {
		val := -5

		result := FormatInt(&val)

		assert.Equal(t, "-5", result)
	})
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{
			name:   "string shorter than max",
			input:  "hello",
			maxLen: 10,
			want:   "hello",
		},
		{
			name:   "string equal to max",
			input:  "hello",
			maxLen: 5,
			want:   "hello",
		},
		{
			name:   "string longer than max",
			input:  "hello world",
			maxLen: 8,
			want:   "hello...",
		},
		{
			name:   "max length less than 4",
			input:  "hello",
			maxLen: 3,
			want:   "hel",
		},
		{
			name:   "max length of 4",
			input:  "hello",
			maxLen: 4,
			want:   "h...",
		},
		{
			name:   "empty string",
			input:  "",
			maxLen: 10,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.want, result)
		})
	}
}
