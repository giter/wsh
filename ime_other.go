//go:build !windows

package main

import "unsafe"

// switchToLatinInput is a no-op off Windows: switching the input method out of
// its native composition mode is a Windows IME concern and has no equivalent
// hooked up on the other platforms.
func switchToLatinInput(_ unsafe.Pointer) {}
