package ui

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/layout"
)

// spacer returns an expanding spacer for right/center alignment in HBox/VBox.
func spacer() fyne.CanvasObject { return layout.NewSpacer() }

// dot renders a small solid colour glyph used to mark a connection.
func dot(c color.Color) *canvas.Text {
	t := canvas.NewText("●", c)
	t.TextSize = 10
	return t
}

// mustInt parses s, returning fallback when it is not a valid integer.
func mustInt(s string, fallback int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}
