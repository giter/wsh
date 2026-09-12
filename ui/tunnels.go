package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"sshclient/storage"
)

// tunnelsView lists port-forwarding rules and lets the user start/stop them.
type tunnelsView struct {
	ui      *App
	list    *fyne.Container
	content *fyne.Container
}

func (ui *App) newTunnelsView() *fyne.Container {
	v := &tunnelsView{ui: ui}
	v.list = container.NewVBox()
	v.rebuild()

	header := widget.NewLabel("隧道")
	header.TextStyle = fyne.TextStyle{Bold: true}

	addBtn := widget.NewButtonWithIcon("新建", theme.ContentAddIcon(), func() {
		v.openEditor(nil)
	})

	v.content = container.NewBorder(
		container.NewHBox(header, spacer(), addBtn),
		nil, nil, nil,
		container.NewPadded(container.NewVScroll(v.list)),
	)
	return v.content
}

func (v *tunnelsView) refresh() {
	v.list.Objects = nil
	v.rebuild()
	v.list.Refresh()
}

func (v *tunnelsView) rebuild() {
	tunnels := v.ui.store.Tunnels()
	if len(tunnels) == 0 {
		empty := widget.NewLabel("还没有隧道。点击上方“新建”转发一个本地端口。")
		empty.Wrapping = fyne.TextWrapWord
		v.list.Add(container.NewCenter(container.NewPadded(empty)))
		return
	}
	for _, t := range tunnels {
		v.list.Add(v.row(t))
	}
}

// connName resolves a connection ID to its display name.
func (v *tunnelsView) connName(id string) string {
	for _, c := range v.ui.store.Connections() {
		if c.ID == id {
			return c.Name
		}
	}
	return id
}

func (v *tunnelsView) row(t *storage.Tunnel) fyne.CanvasObject {
	running := v.ui.tunnels.IsRunning(t.ID)

	title := widget.NewLabel(fmt.Sprintf("%s:%d → %s:%d",
		t.LocalAddress, t.LocalPort, t.RemoteAddress, t.RemotePort))
	title.TextStyle = fyne.TextStyle{Monospace: true}

	sub := widget.NewLabel("通过 " + v.connName(t.ConnectionID))
	sub.TextStyle = fyne.TextStyle{}
	sub.SizeName = theme.SizeNameCaptionText

	state := widget.NewLabel("已停止")
	state.SizeName = theme.SizeNameCaptionText
	if running {
		state.SetText("转发中")
		state.TextStyle = fyne.TextStyle{Bold: true}
	}

	var toggle *widget.Button
	if running {
		toggle = widget.NewButtonWithIcon("停止", theme.MediaStopIcon(), func() {
			v.ui.tunnels.Stop(t.ID)
			v.refresh()
		})
	} else {
		toggle = widget.NewButtonWithIcon("启动", theme.MediaPlayIcon(), func() {
			_ = v.start(t)
			v.refresh()
		})
	}
	editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		v.openEditor(t)
	})
	delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		_ = v.ui.store.DeleteTunnel(t.ID)
		v.ui.tunnels.Stop(t.ID)
		v.refresh()
	})

	row := container.NewHBox(
		dot(ColorAccent),
		container.NewVBox(title, container.NewHBox(sub, spacer(), state)),
		spacer(),
		toggle, editBtn, delBtn,
	)
	return container.NewVBox(
		container.NewPadded(row),
		widget.NewSeparator(),
	)
}

// start launches a tunnel against its connection and reports errors in place.
func (v *tunnelsView) start(t *storage.Tunnel) error {
	c := v.findConn(t.ConnectionID)
	if c == nil {
		return fmt.Errorf("连接不存在")
	}
	_, err := v.ui.tunnels.Start(c, t)
	if err != nil {
		v.showError(err)
	}
	return err
}

func (v *tunnelsView) findConn(id string) *storage.Connection {
	for _, c := range v.ui.store.Connections() {
		if c.ID == id {
			return c
		}
	}
	return nil
}

func (v *tunnelsView) showError(err error) {
	var dlg *widget.PopUp
	dlg = widget.NewModalPopUp(
		container.NewPadded(container.NewVBox(
			widget.NewLabel("隧道启动失败："),
			widget.NewLabel(err.Error()),
			spacer(),
			widget.NewButton("知道了", func() { dlg.Hide() }),
		)),
		v.ui.win.Canvas(),
	)
	dlg.Show()
}

func (v *tunnelsView) openEditor(existing *storage.Tunnel) {
	v.ui.showTunnelEditor(v, existing)
}
