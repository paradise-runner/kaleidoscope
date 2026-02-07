package editor

// IsWordByte determines if a byte is part of a word (non-whitespace)
func IsWordByte(b byte) bool {
	// Treat any non-whitespace byte as a word character so Option/Alt
	// word movements and Option+Delete include punctuation like ',' and '.'.
	return b != ' ' && b != '\t' && b != '\n'
}

// WordLeft moves cursor left to the start of the current/previous word
func WordLeft(line string, col int) int {
	if col <= 0 {
		return 0
	}
	i := col
	// Move left over spaces
	for i > 0 {
		c := line[i-1]
		if c == ' ' || c == '\t' || c == '\n' {
			i--
		} else {
			break
		}
	}
	// Move left over word chars
	for i > 0 && IsWordByte(line[i-1]) {
		i--
	}
	return i
}

// WordRight moves cursor right to the end of the current/next word
func WordRight(line string, col int) int {
	n := len(line)
	if col >= n {
		return n
	}
	i := col
	// If currently on a space, skip spaces
	for i < n {
		c := line[i]
		if c == ' ' || c == '\t' || c == '\n' {
			i++
		} else {
			break
		}
	}
	// If currently at a word, skip the word
	for i < n && IsWordByte(line[i]) {
		i++
	}
	return i
}

// MoveWordLeftLines moves cursor left by word across multiple lines
func MoveWordLeftLines(lines []string, row, col int) (int, int) {
	if row < 0 || row >= len(lines) {
		return row, col
	}
	if col > 0 {
		return row, WordLeft(lines[row], col)
	}
	if row > 0 {
		row--
		return row, WordLeft(lines[row], len(lines[row]))
	}
	return row, col
}

// MoveWordRightLines moves cursor right by word across multiple lines
func MoveWordRightLines(lines []string, row, col int) (int, int) {
	if row < 0 || row >= len(lines) {
		return row, col
	}
	line := lines[row]
	if col < len(line) {
		return row, WordRight(line, col)
	}
	if row < len(lines)-1 {
		row++
		return row, WordRight(lines[row], 0)
	}
	return row, col
}

// DeleteWordBackward deletes from the start of the word to the cursor position
func DeleteWordBackward(line string, col int) (newLine string, newCol int) {
	if col <= 0 {
		return line, col
	}
	newCol = WordLeft(line, col)
	newLine = line[:newCol] + line[col:]
	return newLine, newCol
}
