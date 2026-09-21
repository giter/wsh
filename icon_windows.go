//go:build windows

package main

import (
	"log"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

// The Wails v3 Windows backend registers its window class with
// IDI_APPLICATION — the stock Windows icon — and its own setIcon is a no-op.
// Without the code below the taskbar, Alt+Tab and the window menu show the
// generic Windows icon even though the executable carries icon.ico as a
// resource (wsh.rc, which Explorer and shortcuts use).

var (
	iconMu sync.Mutex
	// iconApplied records the windows that already have the icon, so repeated
	// show events do not allocate a second pair of icons.
	iconApplied = map[uintptr]bool{}
	// windowIcons keeps the HICON handles alive: a window keeps using the
	// handle it was given, so it must not be destroyed while the window lives.
	windowIcons []uintptr
)

// applyWindowIcon makes a window use the application icon.
func applyWindowIcon(win *application.WebviewWindow) {
	if win == nil {
		return
	}
	if applyIcon(win.NativeWindow()) {
		return
	}
	// A window created before app.Run() has no native handle yet: Wails creates
	// it on the main thread while the app starts up, so apply the icon as soon
	// as the window is first shown.
	win.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {
		applyIcon(win.NativeWindow())
	})
}

// applyIcon installs the icon on a native window handle. It reports whether the
// handle was usable, so the caller can tell "done" from "not created yet".
func applyIcon(handle unsafe.Pointer) bool {
	if handle == nil {
		return false
	}
	hwnd := uintptr(handle)

	iconMu.Lock()
	defer iconMu.Unlock()
	if iconApplied[hwnd] {
		return true
	}

	// ICON_BIG is the image the taskbar and Alt+Tab use; ICON_SMALL covers the
	// title bar, the window menu and Alt+Tab's compact view.
	large, err := w32.CreateLargeHIconFromImage(iconICO)
	if err != nil {
		log.Printf("警告：无法创建窗口图标：%v", err)
		return true
	}
	small, err := w32.CreateSmallHIconFromImage(iconICO)
	if err != nil {
		log.Printf("警告：无法创建窗口小图标：%v", err)
		return true
	}
	iconApplied[hwnd] = true
	windowIcons = append(windowIcons, uintptr(large), uintptr(small))
	w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_BIG, uintptr(large))
	w32.SendMessage(hwnd, w32.WM_SETICON, w32.ICON_SMALL, uintptr(small))
	return true
}
