package editor

import (
	"testing"
)

func TestIsWordByte(t *testing.T) {
	tests := []struct {
		name     string
		b        byte
		expected bool
	}{
		{"space", ' ', false},
		{"tab", '\t', false},
		{"newline", '\n', false},
		{"letter", 'a', true},
		{"digit", '1', true},
		{"punctuation", ',', true},
		{"period", '.', true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWordByte(tt.b)
			if result != tt.expected {
				t.Errorf("IsWordByte(%q) = %v, want %v", tt.b, result, tt.expected)
			}
		})
	}
}

func TestWordLeft(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		col      int
		expected int
	}{
		{"at start", "hello world", 0, 0},
		{"in middle of word", "hello world", 3, 0},
		{"after first word", "hello world", 5, 0},
		{"in second word", "hello world", 8, 6},
		{"at end", "hello world", 11, 6},
		{"with punctuation in second word", "hello, world", 9, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WordLeft(tt.line, tt.col)
			if result != tt.expected {
				t.Errorf("WordLeft(%q, %d) = %d, want %d", tt.line, tt.col, result, tt.expected)
			}
		})
	}
}

func TestWordRight(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		col      int
		expected int
	}{
		{"at start", "hello world", 0, 5},
		{"in middle of word", "hello world", 3, 5},
		{"at space", "hello world", 5, 11},
		{"in second word", "hello world", 6, 11},
		{"at end", "hello world", 11, 11},
		{"with punctuation", "hello, world", 0, 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WordRight(tt.line, tt.col)
			if result != tt.expected {
				t.Errorf("WordRight(%q, %d) = %d, want %d", tt.line, tt.col, result, tt.expected)
			}
		})
	}
}

func TestMoveWordLeftLines(t *testing.T) {
	lines := []string{"hello world", "foo bar"}

	tests := []struct {
		name        string
		row, col    int
		expectedRow int
		expectedCol int
	}{
		{"middle of second word", 0, 8, 0, 6},
		{"start of first line", 0, 0, 0, 0},
		{"start of second line wraps to end of first", 1, 0, 0, 6},
		{"middle of first word second line", 1, 2, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, col := MoveWordLeftLines(lines, tt.row, tt.col)
			if row != tt.expectedRow || col != tt.expectedCol {
				t.Errorf("MoveWordLeftLines(%d, %d) = (%d, %d), want (%d, %d)",
					tt.row, tt.col, row, col, tt.expectedRow, tt.expectedCol)
			}
		})
	}
}

func TestMoveWordRightLines(t *testing.T) {
	lines := []string{"hello world", "foo bar"}

	tests := []struct {
		name        string
		row, col    int
		expectedRow int
		expectedCol int
	}{
		{"start of first line", 0, 0, 0, 5},
		{"end of first line", 0, 11, 1, 3},
		{"middle of second line", 1, 0, 1, 3},
		{"end of second line", 1, 7, 1, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, col := MoveWordRightLines(lines, tt.row, tt.col)
			if row != tt.expectedRow || col != tt.expectedCol {
				t.Errorf("MoveWordRightLines(%d, %d) = (%d, %d), want (%d, %d)",
					tt.row, tt.col, row, col, tt.expectedRow, tt.expectedCol)
			}
		})
	}
}

func TestDeleteWordBackward(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		col         int
		expectedLine string
		expectedCol int
	}{
		{"delete word", "hello world", 5, " world", 0},
		{"delete partial word", "hello world", 3, "lo world", 0},
		{"at start", "hello world", 0, "hello world", 0},
		{"delete second word", "hello world", 11, "hello ", 6},
		{"delete from middle of second word", "hello world", 8, "hello rld", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, col := DeleteWordBackward(tt.line, tt.col)
			if line != tt.expectedLine || col != tt.expectedCol {
				t.Errorf("DeleteWordBackward(%q, %d) = (%q, %d), want (%q, %d)",
					tt.line, tt.col, line, col, tt.expectedLine, tt.expectedCol)
			}
		})
	}
}

func TestLineLeft(t *testing.T) {
	lines := []string{"hello world", "foo bar"}

	tests := []struct {
		name        string
		row, col    int
		expectedRow int
		expectedCol int
	}{
		{"middle of line", 0, 5, 0, 0},
		{"start of line", 0, 0, 0, 0},
		{"start of second line", 1, 0, 0, 0},
		{"middle of second line", 1, 3, 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, col := LineLeft(lines, tt.row, tt.col)
			if row != tt.expectedRow || col != tt.expectedCol {
				t.Errorf("LineLeft(%d, %d) = (%d, %d), want (%d, %d)",
					tt.row, tt.col, row, col, tt.expectedRow, tt.expectedCol)
			}
		})
	}
}

func TestLineRight(t *testing.T) {
	lines := []string{"hello world", "foo bar"}

	tests := []struct {
		name        string
		row, col    int
		expectedRow int
		expectedCol int
	}{
		{"middle of line", 0, 5, 0, 11},
		{"end of line", 0, 11, 1, 7},
		{"start of line", 0, 0, 0, 11},
		{"end of last line", 1, 7, 1, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, col := LineRight(lines, tt.row, tt.col)
			if row != tt.expectedRow || col != tt.expectedCol {
				t.Errorf("LineRight(%d, %d) = (%d, %d), want (%d, %d)",
					tt.row, tt.col, row, col, tt.expectedRow, tt.expectedCol)
			}
		})
	}
}

func TestDeleteLineBackward(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		col         int
		expectedLine string
		expectedCol int
	}{
		{"delete from middle", "hello world", 6, "world", 0},
		{"delete all", "hello world", 11, "", 0},
		{"at start", "hello world", 0, "hello world", 0},
		{"partial delete", "hello world", 5, " world", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line, col := DeleteLineBackward(tt.line, tt.col)
			if line != tt.expectedLine || col != tt.expectedCol {
				t.Errorf("DeleteLineBackward(%q, %d) = (%q, %d), want (%q, %d)",
					tt.line, tt.col, line, col, tt.expectedLine, tt.expectedCol)
			}
		})
	}
}

func TestTokenRangesInLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected int // number of tokens expected
	}{
		{"no tokens", "hello world", 0},
		{"one token", "hello [[PASTE#1]] world", 1},
		{"two tokens", "[[PASTE#1]] and [[PASTE#2]]", 2},
		{"token at start", "[[PASTE#1]]hello", 1},
		{"token at end", "hello[[PASTE#1]]", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranges := TokenRangesInLine(tt.line)
			if len(ranges) != tt.expected {
				t.Errorf("TokenRangesInLine(%q) returned %d ranges, want %d", tt.line, len(ranges), tt.expected)
			}
			// Verify ranges are valid
			for _, r := range ranges {
				if r.Start < 0 || r.End > len(tt.line) || r.Start >= r.End {
					t.Errorf("Invalid range: start=%d, end=%d for line length %d", r.Start, r.End, len(tt.line))
				}
				if r.Token != tt.line[r.Start:r.End] {
					t.Errorf("Token mismatch: got %q, expected %q", r.Token, tt.line[r.Start:r.End])
				}
			}
		})
	}
}

func TestTokenRangeContaining(t *testing.T) {
	line := "hello [[PASTE#1]] world"

	tests := []struct {
		name     string
		idx      int
		expected bool
	}{
		{"before token", 4, false},
		{"at token start", 6, true},
		{"in token", 10, true},
		{"at token end - 1", 15, true},
		{"after token", 17, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, ok := TokenRangeContaining(line, tt.idx)
			if ok != tt.expected {
				t.Errorf("TokenRangeContaining(%q, %d) ok = %v, want %v", line, tt.idx, ok, tt.expected)
			}
		})
	}
}

func TestClampCursorOutsideToken(t *testing.T) {
	line := "hello [[PASTE#1]] world"
	// Token is at positions 6-17

	tests := []struct {
		name      string
		col       int
		moveRight bool
		expected  int
	}{
		{"before token", 4, false, 4},
		{"at token start, move left", 6, false, 6},
		{"in token, move left", 10, false, 6},
		{"in token, move right", 10, true, 17},
		{"at token end", 17, false, 17},
		{"after token", 20, false, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClampCursorOutsideToken(line, tt.col, tt.moveRight)
			if result != tt.expected {
				t.Errorf("ClampCursorOutsideToken(%q, %d, %v) = %d, want %d",
					line, tt.col, tt.moveRight, result, tt.expected)
			}
		})
	}
}

func TestIsLongPaste(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		contentWidth int
		expected     bool
	}{
		{"empty", "", 60, false},
		{"one line short", "hello world", 60, false},
		{"one line with many words", "word word word word word word word word word word word word word word word word word word word word word", 60, true},
		{"two lines", "hello\nworld", 60, false},
		{"three lines", "hello\nworld\nfoo", 60, true},
		{"many words narrow width", "one two three four five six seven eight nine ten eleven twelve", 24, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLongPaste(tt.text, tt.contentWidth)
			if result != tt.expected {
				t.Errorf("IsLongPaste(%q, %d) = %v, want %v", tt.text, tt.contentWidth, result, tt.expected)
			}
		})
	}
}
