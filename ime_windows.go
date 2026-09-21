//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// The OS input method is switched through the Win32 IME API because the IME state
// lives in the OS, not the webview: a passphrase prompt cannot ask for English
// from CSS or HTML.
var (
	user32       = syscall.NewLazyDLL("user32.dll")
	procGetFocus = user32.NewProc("GetFocus")

	imm32                      = syscall.NewLazyDLL("imm32.dll")
	procImmGetContext          = imm32.NewProc("ImmGetContext")
	procImmSetConversionStatus = imm32.NewProc("ImmSetConversionStatus")
	procImmReleaseContext      = imm32.NewProc("ImmReleaseContext")
)

// imeCmodeAlphanumeric is the conversion mode for direct (English) input; the
// IME_CMODE_NATIVE bit it clears is what selects the Chinese/Japanese mode.
const imeCmodeAlphanumeric = 0x0000

// switchToLatinInput asks the OS input method to leave its native composition
// mode so the next thing typed is plain ASCII, which is what an SSH passphrase
// needs. It is best-effort: if no IME is active, or the IME does not expose an
// IMM32 context, the calls quietly do nothing and typing is unaffected.
func switchToLatinInput(windowHandle unsafe.Pointer) {
	// The focus sits on the webview's child window rather than the top-level one,
	// so try it first and fall back to the window the shell handed us.
	if hwnd := getFocus(); hwnd != 0 {
		switchLatinInputFor(hwnd)
	}
	if hwnd := uintptr(windowHandle); hwnd != 0 {
		switchLatinInputFor(hwnd)
	}
}

// getFocus returns the window holding the keyboard focus in this thread, or 0
// when the focus lives in another thread.
func getFocus() uintptr {
	hwnd, _, _ := procGetFocus.Call()
	return hwnd
}

// switchLatinInputFor switches a single window's IME context to alphanumeric
// mode.
func switchLatinInputFor(hwnd uintptr) {
	himc, _, _ := procImmGetContext.Call(hwnd)
	if himc == 0 {
		return
	}
	defer procImmReleaseContext.Call(hwnd, himc)
	procImmSetConversionStatus.Call(himc, imeCmodeAlphanumeric, 0)
}
