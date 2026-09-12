package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"sshclient/storage"
)

// connectionsView lists saved servers and offers connect / manage actions.
type connectionsView struct {
	ui      *App
	list    *fyne.Container
	content *fyne.Container
}

func (ui *App) newConnectionsView() *fyne.Container {
	v := &connectionsView{ui: ui}
	v.list = container.NewVBox()
	v.rebuild()

	header := widget.NewLabel("连接")
	header.TextStyle = fyne.TextStyle{Bold: true}
	header.SizeName = theme.SizeNameHeadingText

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

func (v *connectionsView) refresh() {
	v.list.Objects = nil
	v.rebuild()
	v.list.Refresh()
}

func (v *connectionsView) rebuild() {
	conns := v.ui.store.Connections()
	if len(conns) == 0 {
		empty := widget.NewLabel("还没有保存的连接。点击上方“新建”开始。")
		empty.Wrapping = fyne.TextWrapWord
		v.list.Add(container.NewCenter(container.NewPadded(empty)))
		return
	}
	for _, c := range conns {
		v.list.Add(v.row(c))
	}
}

func (v *connectionsView) row(c *storage.Connection) fyne.CanvasObject {
	name := widget.NewLabel(c.Name)
	name.TextStyle = fyne.TextStyle{Bold: true}

	sub := widget.NewLabel(fmt.Sprintf("%s@%s:%d", c.User, c.Host, c.Port))
	sub.TextStyle = fyne.TextStyle{Monospace: true}
	sub.SizeName = theme.SizeNameCaptionText

	connectBtn := widget.NewButtonWithIcon("连接", theme.LoginIcon(), func() {
		v.connect(c)
	})
	editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), func() {
		v.openEditor(c)
	})
	delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		v.remove(c)
	})

	row := container.NewHBox(
		dot(ColorAccent),
		container.NewVBox(name, sub),
		spacer(),
		connectBtn, editBtn, delBtn,
	)
	return container.NewVBox(
		container.NewPadded(row),
		widget.NewSeparator(),
	)
}

func (v *connectionsView) connect(c *storage.Connection) {
	v.ui.openTerminal(c)
}

func (v *connectionsView) remove(c *storage.Connection) {
	confirm := widget.NewModalPopUp(nil, v.ui.win.Canvas())
	body := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("删除连接“%s”？", c.Name)),
		widget.NewLabel("该操作无法撤销。"),
		spacer(),
		container.NewHBox(
			widget.NewButton("取消", func() { confirm.Hide() }),
			widget.NewButtonWithIcon("删除", theme.DeleteIcon(), func() {
				_ = v.ui.store.DeleteConnection(c.ID)
				confirm.Hide()
				v.refresh()
			}),
		),
	)
	confirm.Content = container.NewPadded(body)
	confirm.Show()
}

// openEditor shows the add/edit dialog.
func (v *connectionsView) openEditor(existing *storage.Connection) {
	v.ui.showConnectionEditor(v, existing)
}
