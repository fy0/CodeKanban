package terminal

import "github.com/charmbracelet/x/xpty"

type terminalSize struct {
	cols int
	rows int
}

func resizeSequence(goos string, currentCols, currentRows, targetCols, targetRows int) []terminalSize {
	if targetCols <= 0 || targetRows <= 0 {
		return nil
	}
	// Some Unix shells only repaint after a real window-size change. A
	// same-size TIOCSWINSZ can be ignored by the kernel, so briefly nudge the
	// row count to force SIGWINCH and immediately restore the requested size.
	if shouldNudgeSameSizeResize(goos) && currentCols == targetCols && currentRows == targetRows {
		return []terminalSize{{cols: targetCols, rows: targetRows + 1}, {cols: targetCols, rows: targetRows}}
	}
	return []terminalSize{{cols: targetCols, rows: targetRows}}
}

func shouldNudgeSameSizeResize(goos string) bool {
	switch goos {
	case "linux", "darwin":
		return true
	default:
		return false
	}
}

func resizePTYForTarget(pty xpty.Pty, goos string, currentCols, currentRows, targetCols, targetRows int) error {
	for _, size := range resizeSequence(goos, currentCols, currentRows, targetCols, targetRows) {
		if err := pty.Resize(size.cols, size.rows); err != nil {
			return err
		}
	}
	return nil
}
