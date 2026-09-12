package ui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"sshclient/storage"
)

// showTunnelEditor opens a window for creating or editing a port-forward rule.
func (ui *App) showTunnelEditor(listView *tunnelsView, existing *storage.Tunnel) {
	w := ui.app.NewWindow("隧道")
	w.Resize(fyne.NewSize(420, 420))

	conns := ui.store.Connections()
	names := make([]string, 0, len(conns))
	byName := map[string]*storage.Connection{}
	for _, c := range conns {
		names = append(names, c.Name)
		byName[c.Name] = c
	}
	if len(names) == 0 {
		names = append(names, "(无连接)")
	}

	name := widget.NewEntry()
	connSel := widget.NewSelect(names, nil)
	connSel.SetSelectedIndex(0)
	localAddr := widget.NewEntry()
	localAddr.SetPlaceHolder("127.0.0.1")
	localPort := widget.NewEntry()
	localPort.SetText("8080")
	remoteAddr := widget.NewEntry()
	remoteAddr.SetPlaceHolder("127.0.0.1")
	remotePort := widget.NewEntry()
	remotePort.SetText("80")

	if existing != nil {
		w.SetTitle("编辑隧道")
		name.SetText(existing.Name)
		if byName != nil {
			for _, c := range conns {
				if c.ID == existing.ConnectionID {
					connSel.SetSelected(c.Name)
					break
				}
			}
		}
		localAddr.SetText(existing.LocalAddress)
		localPort.SetText(strconv.Itoa(existing.LocalPort))
		remoteAddr.SetText(existing.RemoteAddress)
		remotePort.SetText(strconv.Itoa(existing.RemotePort))
	}

	status := widget.NewLabel("")
	status.TextStyle = fyne.TextStyle{Italic: true}

	save := func() {
		if name.Text == "" {
			status.SetText("请填写隧道名称。")
			return
		}
		lp, err1 := strconv.Atoi(localPort.Text)
		rp, err2 := strconv.Atoi(remotePort.Text)
		if err1 != nil || err2 != nil || lp < 1 || lp > 65535 || rp < 1 || rp > 65535 {
			status.SetText("端口必须是 1-65535 之间的数字。")
			return
		}
		if localAddr.Text == "" {
			localAddr.SetText("127.0.0.1")
		}
		if remoteAddr.Text == "" {
			remoteAddr.SetText("127.0.0.1")
		}

		var cid string
		if byName[connSel.Selected] != nil {
			cid = byName[connSel.Selected].ID
		}

		if existing == nil {
			t := &storage.Tunnel{
				ID:            storage.NewID(),
				Name:          name.Text,
				ConnectionID:  cid,
				LocalAddress:  localAddr.Text,
				LocalPort:     lp,
				RemoteAddress: remoteAddr.Text,
				RemotePort:    rp,
			}
			if err := ui.store.AddTunnel(t); err != nil {
				status.SetText("保存失败：" + err.Error())
				return
			}
		} else {
			existing.Name = name.Text
			existing.ConnectionID = cid
			existing.LocalAddress = localAddr.Text
			existing.LocalPort = lp
			existing.RemoteAddress = remoteAddr.Text
			existing.RemotePort = rp
			if err := ui.store.UpdateTunnel(existing); err != nil {
				status.SetText("保存失败：" + err.Error())
				return
			}
		}
		if listView != nil {
			listView.refresh()
		}
		w.Close()
	}

	form := container.NewVBox(
		widget.NewLabel("名称"),
		name,
		widget.NewLabel("连接"),
		connSel,
		widget.NewLabel("本地地址"),
		localAddr,
		widget.NewLabel("本地端口"),
		localPort,
		widget.NewLabel("远程地址"),
		remoteAddr,
		widget.NewLabel("远程端口"),
		remotePort,
		status,
		spacer(),
		container.NewHBox(
			widget.NewButton("取消", func() { w.Close() }),
			widget.NewButtonWithIcon("保存", nil, save),
		),
	)

	w.SetContent(container.NewPadded(container.NewVScroll(form)))
	w.Show()
}
