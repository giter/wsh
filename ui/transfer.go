package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	sshclient "sshclient/ssh"
	"sshclient/storage"
)

// transferView provides a two-pane SFTP browser for moving files.
type transferView struct {
	ui      *App
	content *fyne.Container

	connSel *widget.Select
	status  *widget.Label

	localPath  string
	remotePath string
	localList  *fyne.Container
	remoteList *fyne.Container
	localLbl   *widget.Label
	remoteLbl  *widget.Label

	activeConn *storage.Connection
	client     *ssh.Client
}

func (ui *App) newTransferView() *fyne.Container {
	v := &transferView{ui: ui}

	conns := ui.store.Connections()
	names := make([]string, 0, len(conns))
	byName := map[string]*storage.Connection{}
	for _, c := range conns {
		names = append(names, c.Name)
		byName[c.Name] = c
	}

	v.connSel = widget.NewSelect(names, func(name string) {
		v.activeConn = byName[name]
		v.status.SetText("正在连接…")
		go v.connectRemote()
	})

	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/"
	}
	v.localPath = home

	v.localLbl = widget.NewLabel(v.localPath)
	v.localLbl.TextStyle = fyne.TextStyle{Monospace: true}
	v.localLbl.SizeName = theme.SizeNameCaptionText
	v.remoteLbl = widget.NewLabel("未连接")
	v.remoteLbl.TextStyle = fyne.TextStyle{Monospace: true}
	v.remoteLbl.SizeName = theme.SizeNameCaptionText

	v.localList = container.NewVBox()
	v.remoteList = container.NewVBox()

	upLocal := widget.NewButtonWithIcon("上级", theme.MediaSkipPreviousIcon(), func() {
		v.navigateLocal("..")
	})
	upRemote := widget.NewButtonWithIcon("上级", theme.MediaSkipPreviousIcon(), func() {
		v.navigateRemote("..")
	})

	localPanel := container.NewBorder(
		container.NewVBox(
			container.NewHBox(widget.NewLabel("本地"), spacer(), upLocal),
			v.localLbl,
		),
		nil, nil, nil,
		container.NewPadded(container.NewVScroll(v.localList)),
	)
	remotePanel := container.NewBorder(
		container.NewVBox(
			container.NewHBox(widget.NewLabel("远程"), spacer(), upRemote),
			v.remoteLbl,
		),
		nil, nil, nil,
		container.NewPadded(container.NewVScroll(v.remoteList)),
	)

	split := container.NewHSplit(localPanel, remotePanel)
	split.SetOffset(0.5)

	v.status = widget.NewLabel("选择连接以浏览远程文件。")
	v.status.TextStyle = fyne.TextStyle{Italic: true}

	v.content = container.NewBorder(
		container.NewVBox(
			container.NewHBox(widget.NewLabel("连接"), v.connSel),
		),
		container.NewPadded(v.status),
		nil, nil,
		split,
	)

	v.reloadLocal()
	return v.content
}

func (v *transferView) reloadLocal() {
	v.localList.Objects = nil
	entries, err := os.ReadDir(v.localPath)
	if err != nil {
		v.localList.Add(widget.NewLabel("无法读取：" + err.Error()))
		v.localList.Refresh()
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, e := range entries {
		e := e
		name := e.Name()
		if e.IsDir() {
			v.localList.Add(v.localDirRow(name))
		} else {
			v.localList.Add(v.localFileRow(name, e))
		}
	}
	v.localLbl.SetText(v.localPath)
	v.localList.Refresh()
}

func (v *transferView) localDirRow(name string) fyne.CanvasObject {
	b := widget.NewButtonWithIcon(name+"/", theme.FolderIcon(), func() {
		v.navigateLocal(name)
	})
	b.Alignment = widget.ButtonAlignLeading
	return container.NewPadded(b)
}

func (v *transferView) localFileRow(name string, info os.DirEntry) fyne.CanvasObject {
	row := container.NewHBox(
		widget.NewIcon(theme.FileIcon()),
		widget.NewLabel(name),
		spacer(),
		widget.NewButtonWithIcon("上传", theme.MediaPlayIcon(), func() {
			v.upload(name)
		}),
	)
	_ = info
	return container.NewPadded(row)
}

func (v *transferView) navigateLocal(name string) {
	target := filepath.Join(v.localPath, name)
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return
	}
	v.localPath = target
	v.reloadLocal()
}

// ---- remote side ----

func (v *transferView) connectRemote() {
	if v.activeConn == nil {
		return
	}
	client, err := v.ui.pool.Get(v.activeConn, nil)
	if err != nil {
		fyne.Do(func() {
			v.status.SetText("连接失败：" + err.Error())
			v.remoteList.Objects = nil
			v.remoteList.Refresh()
		})
		return
	}
	fyne.Do(func() {
		v.client = client
		v.remotePath = "/"
		v.reloadRemote()
	})
}

func (v *transferView) reloadRemote() {
	if v.client == nil {
		v.remoteList.Objects = nil
		v.remoteLbl.SetText("未连接")
		v.remoteList.Refresh()
		return
	}
	v.remoteList.Objects = nil
	c, err := sftp.NewClient(v.client)
	if err != nil {
		v.remoteList.Add(widget.NewLabel("SFTP 错误：" + err.Error()))
		v.remoteList.Refresh()
		return
	}
	defer c.Close()
	entries, err := c.ReadDir(v.remotePath)
	if err != nil {
		v.remoteList.Add(widget.NewLabel("无法读取：" + err.Error()))
		v.remoteLbl.SetText(v.remotePath)
		v.remoteList.Refresh()
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			b := widget.NewButtonWithIcon(name+"/", theme.FolderIcon(), func() {
				v.navigateRemote(name)
			})
			b.Alignment = widget.ButtonAlignLeading
			v.remoteList.Add(container.NewPadded(b))
		} else {
			row := container.NewHBox(
				widget.NewIcon(theme.FileIcon()),
				widget.NewLabel(name),
				spacer(),
				widget.NewButtonWithIcon("下载", theme.MediaPlayIcon(), func() {
					v.download(name)
				}),
			)
			v.remoteList.Add(container.NewPadded(row))
		}
	}
	v.remoteLbl.SetText(v.remotePath)
	v.remoteList.Refresh()
}

func (v *transferView) navigateRemote(name string) {
	if name == ".." {
		if v.remotePath == "/" {
			return
		}
		idx := strings.LastIndex(v.remotePath, "/")
		if idx <= 0 {
			v.remotePath = "/"
		} else {
			v.remotePath = v.remotePath[:idx]
		}
	} else {
		v.remotePath = strings.TrimRight(v.remotePath, "/") + "/" + name
	}
	v.reloadRemote()
}

func (v *transferView) upload(name string) {
	if v.client == nil {
		v.status.SetText("请先选择连接。")
		return
	}
	local := filepath.Join(v.localPath, name)
	remote := strings.TrimRight(v.remotePath, "/") + "/" + name
	v.status.SetText("上传 " + name + " …")
	go func() {
		res := sshclient.TransferFile(v.client, sshclient.Upload, local, remote)
		fyne.Do(func() {
			if res.Err != nil {
				v.status.SetText("上传失败：" + res.Err.Error())
			} else {
				v.status.SetText(fmt.Sprintf("已上传 %s（%d 字节）", name, res.Bytes))
			}
			v.reloadRemote()
		})
	}()
}

func (v *transferView) download(name string) {
	if v.client == nil {
		return
	}
	remote := strings.TrimRight(v.remotePath, "/") + "/" + name
	local := filepath.Join(v.localPath, name)
	v.status.SetText("下载 " + name + " …")
	go func() {
		res := sshclient.TransferFile(v.client, sshclient.Download, local, remote)
		fyne.Do(func() {
			if res.Err != nil {
				v.status.SetText("下载失败：" + res.Err.Error())
			} else {
				v.status.SetText(fmt.Sprintf("已下载 %s（%d 字节）", name, res.Bytes))
			}
			v.reloadLocal()
		})
	}()
}
