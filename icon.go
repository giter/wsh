package main

import _ "embed"

// iconICO is the application icon. The same file is compiled into the
// executable as a Windows resource (wsh.rc) for Explorer and shortcuts, and
// embedded here so the runtime can set the window icon, which the Wails
// Windows backend does not do.
//
//go:embed build/icon.ico
var iconICO []byte
