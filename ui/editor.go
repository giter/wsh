package ui

import (
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	sshclient "sshclient/ssh"
	"sshclient/storage"
)

// showConnectionEditor opens a window for creating or editing a connection.
// When existing is nil a new record is created.
func (ui *App) showConnectionEditor(listView *connectionsView, existing *storage.Connection) {
	w := ui.app.NewWindow("连接")
	w.Resize(fyne.NewSize(420, 460))

	name := widget.NewEntry()
	host := widget.NewEntry()
	host.SetPlaceHolder("example.com")
	port := widget.NewEntry()
	port.SetText("22")
	user := widget.NewEntry()
	user.SetPlaceHolder("root")
	password := widget.NewEntry()
	password.Password = true
	savePw := widget.NewCheck("保存密码", nil)
	keyPath := widget.NewEntry()
	keyPath.SetPlaceHolder("可选：/path/to/id_rsa")

	if existing != nil {
		w.SetTitle("编辑连接")
		name.SetText(existing.Name)
		host.SetText(existing.Host)
		port.SetText(strconv.Itoa(existing.Port))
		user.SetText(existing.User)
		savePw.SetChecked(existing.SavePassword)
		keyPath.SetText(existing.PrivateKeyPath)
		if existing.SavePassword {
			if dec, err := storage.DecryptPassword(existing.EncryptedPassword); err == nil {
				password.SetText(dec)
			}
		}
	} else {
		port.SetText("22")
	}

	status := widget.NewLabel("")
	status.TextStyle = fyne.TextStyle{Italic: true}

	save := func() {
		if name.Text == "" || host.Text == "" || user.Text == "" {
			status.SetText("名称、主机和用户不能为空。")
			return
		}
		p, err := strconv.Atoi(port.Text)
		if err != nil || p < 1 || p > 65535 {
			status.SetText("端口必须是 1-65535 之间的数字。")
			return
		}

		var enc string
		if savePw.Checked && password.Text != "" {
			enc, err = storage.EncryptPassword(password.Text)
			if err != nil {
				status.SetText("密码保存失败：" + err.Error())
				return
			}
		}

		if existing == nil {
			c := &storage.Connection{
				ID:                storage.NewID(),
				Name:              name.Text,
				Host:              host.Text,
				Port:              p,
				User:              user.Text,
				EncryptedPassword: enc,
				SavePassword:      savePw.Checked,
				PrivateKeyPath:    keyPath.Text,
				Color:             "#34D399",
			}
			if err := ui.store.AddConnection(c); err != nil {
				status.SetText("保存失败：" + err.Error())
				return
			}
		} else {
			existing.Name = name.Text
			existing.Host = host.Text
			existing.Port = p
			existing.User = user.Text
			existing.EncryptedPassword = enc
			existing.SavePassword = savePw.Checked
			existing.PrivateKeyPath = keyPath.Text
			if err := ui.store.UpdateConnection(existing); err != nil {
				status.SetText("保存失败：" + err.Error())
				return
			}
		}
		if listView != nil {
			listView.refresh()
		}
		w.Close()
	}

	testBtn := widget.NewButtonWithIcon("测试连接", theme.ComputerIcon(), func() {
		status.SetText("测试中…")
		tmp := &storage.Connection{
			Name:           name.Text,
			Host:           host.Text,
			Port:           mustInt(port.Text, 22),
			User:           user.Text,
			PrivateKeyPath: keyPath.Text,
		}
		go func() {
			client, err := sshclient.Dial(tmp, password.Text)
			fyne.Do(func() {
				if err != nil {
					status.SetText("连接失败：" + err.Error())
					return
				}
				status.SetText("连接成功")
			})
			if client != nil {
				_ = client.Close()
			}
		}()
	})

	form := container.NewVBox(
		widget.NewLabel("名称"),
		name,
		widget.NewLabel("主机"),
		host,
		widget.NewLabel("端口"),
		port,
		widget.NewLabel("用户"),
		user,
		widget.NewLabel("密码"),
		password,
		savePw,
		widget.NewLabel("私钥路径"),
		keyPath,
		status,
		spacer(),
		container.NewHBox(
			widget.NewButton("取消", func() { w.Close() }),
			testBtn,
			widget.NewButtonWithIcon("保存", theme.ConfirmIcon(), save),
		),
	)

	w.SetContent(container.NewPadded(container.NewVScroll(form)))
	w.Show()
}
