package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/sftp"
)

type sftpEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
}

type sftpListParams struct {
	ConnectionID string `json:"connId"`
	Path         string `json:"path"`

	// Credentials typed at a connect prompt. They are only sent when the stored
	// connection has no usable password (or its saved one was rejected).
	Password      string `json:"password,omitempty"`
	KeyPassphrase string `json:"keyPassphrase,omitempty"`
}

func (s *Server) handleSftpList(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p sftpListParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	client, prompt, err := s.sftpClient(p.ConnectionID, p.Password, p.KeyPassphrase)
	if err != nil {
		return nil, err
	}
	if prompt != nil {
		return prompt, nil
	}
	defer client.Close()

	if p.Path == "" {
		p.Path = "/"
	}
	infos, err := client.ReadDir(p.Path)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败：%w", err)
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].IsDir() != infos[j].IsDir() {
			return infos[i].IsDir()
		}
		return infos[i].Name() < infos[j].Name()
	})
	out := make([]sftpEntry, 0, len(infos))
	for _, fi := range infos {
		out = append(out, sftpEntry{
			Name:  fi.Name(),
			IsDir: fi.IsDir(),
			Size:  fi.Size(),
			Mode:  fi.Mode().String(),
		})
	}
	return map[string]interface{}{"path": p.Path, "entries": out}, nil
}

type sftpUploadParams struct {
	ConnectionID string `json:"connId"`
	RemoteDir    string `json:"remoteDir"`
	Name         string `json:"name"`
	Data         string `json:"data"` // base64

	Password      string `json:"password,omitempty"`
	KeyPassphrase string `json:"keyPassphrase,omitempty"`
}

func (s *Server) handleSftpUpload(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p sftpUploadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	raw, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return nil, fmt.Errorf("解码文件失败：%w", err)
	}
	client, prompt, err := s.sftpClient(p.ConnectionID, p.Password, p.KeyPassphrase)
	if err != nil {
		return nil, err
	}
	if prompt != nil {
		return prompt, nil
	}
	defer client.Close()

	remote := strings.TrimRight(p.RemoteDir, "/") + "/" + p.Name
	f, err := client.Create(remote)
	if err != nil {
		return nil, fmt.Errorf("创建远程文件失败：%w", err)
	}
	_, werr := f.Write(raw)
	cerr := f.Close()
	if werr != nil {
		return nil, fmt.Errorf("写入远程文件失败：%w", werr)
	}
	if cerr != nil {
		return nil, fmt.Errorf("关闭远程文件失败：%w", cerr)
	}
	return map[string]interface{}{"name": p.Name, "bytes": len(raw)}, nil
}

type sftpDownloadParams struct {
	ConnectionID string `json:"connId"`
	Path         string `json:"path"`

	Password      string `json:"password,omitempty"`
	KeyPassphrase string `json:"keyPassphrase,omitempty"`
}

func (s *Server) handleSftpDownload(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p sftpDownloadParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	client, prompt, err := s.sftpClient(p.ConnectionID, p.Password, p.KeyPassphrase)
	if err != nil {
		return nil, err
	}
	if prompt != nil {
		return prompt, nil
	}
	defer client.Close()

	f, err := client.Open(p.Path)
	if err != nil {
		return nil, fmt.Errorf("打开远程文件失败：%w", err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	data := make([]byte, fi.Size())
	if _, err := f.Read(data); err != nil {
		return nil, fmt.Errorf("读取远程文件失败：%w", err)
	}
	name := filepath.Base(p.Path)
	return map[string]interface{}{
		"name": name,
		"size": fi.Size(),
		"data": base64.StdEncoding.EncodeToString(data),
	}, nil
}

type sftpMkdirParams struct {
	ConnectionID string `json:"connId"`
	Parent       string `json:"parent"`
	Name         string `json:"name"`

	Password      string `json:"password,omitempty"`
	KeyPassphrase string `json:"keyPassphrase,omitempty"`
}

func (s *Server) handleSftpMkdir(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p sftpMkdirParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	client, prompt, err := s.sftpClient(p.ConnectionID, p.Password, p.KeyPassphrase)
	if err != nil {
		return nil, err
	}
	if prompt != nil {
		return prompt, nil
	}
	defer client.Close()

	dir := strings.TrimRight(p.Parent, "/") + "/" + p.Name
	if err := client.MkdirAll(dir); err != nil {
		return nil, fmt.Errorf("创建目录失败：%w", err)
	}
	return nil, nil
}

// sftpClient returns a fresh SFTP client over the pooled connection.
//
// When the connection has no usable credentials the dial fails; instead of
// surfacing a dead-end authentication error the caller receives a non-nil
// prompt map ({needPassword} or {needPassphrase}) that the UI turns into a
// prompt, retrying the call with the typed credential.
func (s *Server) sftpClient(connID, password, keyPassphrase string) (*sftp.Client, map[string]interface{}, error) {
	conn, err := s.findConnection(connID)
	if err != nil {
		return nil, nil, err
	}
	if keyPassphrase != "" && conn.KeyID != "" {
		s.pool.ProvidePassphrase(conn.KeyID, keyPassphrase)
	}
	var passPtr *string
	if password != "" {
		passPtr = &password
	}
	client, err := s.pool.Get(conn, passPtr)
	if err != nil {
		if prompt, ok := s.credentialPrompt(err); ok {
			return nil, prompt, nil
		}
		return nil, nil, err
	}
	sc, err := sftp.NewClient(client)
	if err != nil {
		return nil, nil, fmt.Errorf("SFTP 连接失败：%w", err)
	}
	return sc, nil, nil
}
