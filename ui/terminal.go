package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/fyne-io/terminal"

	sshclient "sshclient/ssh"
	"sshclient/storage"
)

// openTerminal dials the connection and opens an SSH terminal in a new window.
func (ui *App) openTerminal(c *storage.Connection) {
	term := terminal.New()

	w := ui.app.NewWindow(fmt.Sprintf("%s · %s@%s", c.Name, c.User, c.Host))
	w.Resize(fyne.NewSize(900, 600))

	status := widget.NewLabel("正在连接 " + c.Host + " …")
	status.TextStyle = fyne.TextStyle{Italic: true}
	connecting := container.NewCenter(container.NewPadded(status))
	w.SetContent(connecting)
	w.Show()

	var connect func(string)
	connect = func(password string) {
		status.SetText("正在连接 " + c.Host + " …")
		go func() {
			client, err := ui.pool.Get(c, &password)
			if err != nil {
				fyne.Do(func() { ui.failConnect(w, c, err, connect) })
				return
			}
			ts := sshclient.NewTerminalSession(client, term)
			ts.OnExit = func() {
				fyne.Do(func() { w.Close() })
				ui.removeSession(idOf(c, term))
			}
			if err := ts.Start(); err != nil {
				fyne.Do(func() { ui.failConnect(w, c, err, connect) })
				return
			}
			ui.addSession(idOf(c, term), ts)
			fyne.Do(func() {
				w.SetContent(term)
				w.SetOnClosed(func() {
					ts.Close()
					ui.removeSession(idOf(c, term))
				})
			})
		}()
	}

	connect("")
}

// idOf builds a stable session key for a connection+terminal pair.
func idOf(c *storage.Connection, term *terminal.Terminal) string {
	return fmt.Sprintf("%s/%p", c.ID, term)
}

func (ui *App) failConnect(w fyne.Window, c *storage.Connection, err error, retry func(string)) {
	pw := widget.NewEntry()
	pw.Password = true

	var dlg *widget.PopUp
	actions := container.NewHBox(
		widget.NewButton("关闭", func() { dlg.Hide(); w.Close() }),
		widget.NewButton("重试", func() {
			dlg.Hide()
			ui.pool.ProvidePassword(c.ID, pw.Text)
			retry(pw.Text)
		}),
	)

	body := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("连接 %s 失败：", c.Host)),
		widget.NewLabel(err.Error()),
		widget.NewLabel("输入密码重试，或检查网络和服务配置。"),
		pw,
		spacer(),
		actions,
	)
	dlg = widget.NewModalPopUp(container.NewPadded(body), w.Canvas())
	dlg.Show()
}
