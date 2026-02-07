package editor

import (
	"strings"
)

// Paste placeholder token markers
const PasteTokenPrefix = "[[PASTE#"
const PasteTokenSuffix = "]]"

// TokenRange represents a paste token's location and value in a line
type TokenRange struct {
	Start int
	End   int
	Token string
}

// TokenRangesInLine finds all paste tokens in a line and returns their byte ranges
func TokenRangesInLine(line string) []TokenRange {
	var out []TokenRange
	search := line
	base := 0
	for {
		i := strings.Index(search, PasteTokenPrefix)
		if i < 0 {
			break
		}
		start := base + i
		j := strings.Index(search[i:], PasteTokenSuffix)
		if j < 0 {
			break
		}
		end := start + j + len(PasteTokenSuffix)
		// extract token string
		tok := line[start:end]
		out = append(out, TokenRange{
			Start: start,
			End:   end,
			Token: tok,
		})
		// advance
		advance := i + j + len(PasteTokenSuffix)
		base += advance
		if advance >= len(search) {
			break
		}
		search = search[advance:]
	}
	return out
}

// TokenRangeContaining returns the token range that contains index idx, if any
func TokenRangeContaining(line string, idx int) (start, end int, token string, ok bool) {
	for _, r := range TokenRangesInLine(line) {
		if idx >= r.Start && idx < r.End {
			return r.Start, r.End, r.Token, true
		}
	}
	return 0, 0, "", false
}

// ClampCursorOutsideToken moves col to token boundary if inside a token
func ClampCursorOutsideToken(line string, col int, moveRight bool) int {
	if col < 0 {
		return 0
	}
	if col > len(line) {
		return len(line)
	}
	if start, end, _, ok := TokenRangeContaining(line, col); ok {
		if moveRight {
			return end
		}
		return start
	}
	return col
}

// IsLongPaste determines if pasted text should be collapsed.
// Heuristic: collapse if there are >2 explicit lines, or if word-wrapped
// into ~contentWidth/6 words per line would exceed 2 lines.
func IsLongPaste(text string, contentWidth int) bool {
	if text == "" {
		return false
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 2 {
		return true
	}
	// Words-based approximation
	words := len(strings.Fields(text))
	wordsPerLine := contentWidth / 6
	if wordsPerLine < 1 {
		wordsPerLine = 1
	}
	approxLines := (words + wordsPerLine - 1) / wordsPerLine
	return approxLines > 2
}
