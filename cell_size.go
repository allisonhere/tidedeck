package tideui

import (
	"os"

	"golang.org/x/sys/unix"
)

// CellAspectOf measures a terminal: the height of one cell divided by its width.
// It reads the window size the terminal reports in pixels in the same ioctl that
// reports it in cells, which is the only portable way to learn the shape of a
// cell - and the only thing a picture drawn in cells needs to keep its
// proportions. Most fonts land near 2 (a 7x15 cell, an 8x17 cell), which is what
// a zero here means: a pty with no emulator behind it, a window whose pixel size
// is not known yet, or a terminal that does not report pixels at all.
func CellAspectOf(file *os.File) float64 {
	if file == nil {
		return 0
	}
	size, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	if err != nil || size == nil || size.Col == 0 || size.Row == 0 {
		return 0
	}
	if size.Xpixel == 0 || size.Ypixel == 0 {
		return 0
	}
	width := float64(size.Xpixel) / float64(size.Col)
	height := float64(size.Ypixel) / float64(size.Row)
	if width <= 0 || height <= 0 {
		return 0
	}
	return height / width
}
