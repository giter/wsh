//go:build !windows

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// applyWindowIcon is a no-op where the window icon comes from the application
// bundle (macOS) or the desktop entry (Linux) instead of being set per window.
func applyWindowIcon(*application.WebviewWindow) {}
