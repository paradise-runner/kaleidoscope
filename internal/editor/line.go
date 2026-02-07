package editor

// LineLeft jumps to the start of current line, or to the previous line if already at start
func LineLeft(lines []string, row, col int) (int, int) {
	if row < 0 || row >= len(lines) {
		return row, col
	}
	if col > 0 {
		return row, 0
	}
	if row > 0 {
		return row - 1, 0
	}
	return row, col
}

// LineRight jumps to the end of current line, or to the next line if already at end
func LineRight(lines []string, row, col int) (int, int) {
	if row < 0 || row >= len(lines) {
		return row, col
	}
	lineLen := len(lines[row])
	if col < lineLen {
		return row, lineLen
	}
	if row < len(lines)-1 {
		row++
		return row, len(lines[row])
	}
	return row, col
}

// DeleteLineBackward deletes from the start of the line to the cursor position
func DeleteLineBackward(line string, col int) (newLine string, newCol int) {
	if col <= 0 {
		return line, col
	}
	newLine = line[col:]
	return newLine, 0
}
