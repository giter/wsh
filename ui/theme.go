package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Palette. One accent (emerald, the colour of a live connection) does the
// talking; everything else stays quiet so the connection data reads first.
var (
	// Surfaces
	ColorBg       = color.NRGBA{R: 0x0F, G: 0x11, B: 0x15, A: 0xFF}
	ColorPanel    = color.NRGBA{R: 0x17, G: 0x1A, B: 0x20, A: 0xFF}
	ColorElevated = color.NRGBA{R: 0x1F, G: 0x23, B: 0x2B, A: 0xFF}
	ColorBorder   = color.NRGBA{R: 0x2A, G: 0x30, B: 0x3B, A: 0xFF}

	// Text
	ColorText    = color.NRGBA{R: 0x9A, G: 0xA3, B: 0xB2, A: 0xFF}
	ColorTextDim = color.NRGBA{R: 0x5B, G: 0x64, B: 0x72, A: 0xFF}

	// Accent + status
	ColorAccent   = color.NRGBA{R: 0x34, G: 0xD3, B: 0x99, A: 0xFF}
	ColorOnAccent = color.NRGBA{R: 0x06, G: 0x0E, B: 0x0A, A: 0xFF}
	ColorDanger   = color.NRGBA{R: 0xF4, G: 0x3F, B: 0x5E, A: 0xFF}
)

// AppTheme implements fyne.Theme with a terminal-inspired dark identity.
type AppTheme struct{}

// Color resolves a theme colour name.
func (t *AppTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return ColorBg
	case theme.ColorNameInputBackground:
		return ColorPanel
	case theme.ColorNameHover:
		return ColorElevated
	case theme.ColorNameDisabledButton:
		return ColorElevated
	case theme.ColorNameFocus:
		return ColorAccent
	case theme.ColorNamePrimary:
		return ColorAccent
	case theme.ColorNameButton:
		return ColorElevated
	case theme.ColorNameDisabled:
		return ColorTextDim
	case theme.ColorNameForeground:
		return ColorText
	case theme.ColorNamePlaceHolder:
		return ColorTextDim
	case theme.ColorNameScrollBar:
		return ColorTextDim
	case theme.ColorNameSelection:
		return ColorAccent
	case theme.ColorNamePressed:
		return ColorElevated
	case theme.ColorNameMenuBackground:
		return ColorPanel
	case theme.ColorNameShadow:
		return color.Transparent
	case theme.ColorNameSeparator:
		return ColorBorder
	case theme.ColorNameOverlayBackground:
		return ColorPanel
	case theme.ColorNameHeaderBackground:
		return ColorPanel
	case theme.ColorNameSuccess:
		return ColorAccent
	case theme.ColorNameError:
		return ColorDanger
	case theme.ColorNameWarning:
		return color.NRGBA{R: 0xFB, G: 0xBD, B: 0x50, A: 0xFF}
	default:
		return theme.DarkTheme().Color(name, variant)
	}
}

// Font returns a font resource. Defaults keep system fonts for legibility.
func (t *AppTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DarkTheme().Font(style)
}

// Icon resolves a theme icon.
func (t *AppTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DarkTheme().Icon(name)
}

// Size resolves a theme size value.
func (t *AppTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameInnerPadding:
		return 10
	case theme.SizeNameInputBorder:
		return 0
	case theme.SizeNameInlineIcon:
		return 18
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameText:
		return 13
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameSubHeadingText:
		return 15
	case theme.SizeNameCaptionText:
		return 11
	default:
		return theme.DarkTheme().Size(name)
	}
}
