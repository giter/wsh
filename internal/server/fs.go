package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// fsEntry mirrors a local directory entry for the SFTP local pane.
type fsEntry struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

func (s *Server) handleFsList(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if p.Path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		p.Path = home
	}
	entries, err := os.ReadDir(p.Path)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败：%w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	out := make([]fsEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, fsEntry{Name: e.Name(), IsDir: e.IsDir(), Size: info.Size()})
	}
	abs, _ := filepath.Abs(p.Path)
	return map[string]interface{}{"path": abs, "entries": out}, nil
}
