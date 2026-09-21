package server

import (
	"encoding/json"
	"fmt"
	"strings"

	"sshclient/storage"
)

// folderView is the folder shape sent to the browser.
type folderView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`
}

func (s *Server) handleListFolders(c *wsClient, params json.RawMessage) (interface{}, error) {
	fs := s.store.Folders()
	out := make([]folderView, 0, len(fs))
	for _, f := range fs {
		out = append(out, folderView{ID: f.ID, Name: f.Name, Order: f.Order})
	}
	return out, nil
}

// saveFolderParams creates (id empty) or renames a folder.
type saveFolderParams struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Server) handleSaveFolder(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p saveFolderParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return nil, fmt.Errorf("请输入文件夹名称")
	}
	if p.ID == "" {
		f := &storage.Folder{ID: storage.NewID(), Name: p.Name}
		if err := s.store.AddFolder(f); err != nil {
			return nil, err
		}
		// New folders go last, after any the user has already arranged.
		ids := make([]string, 0, len(s.store.Folders()))
		for _, existing := range s.store.Folders() {
			if existing.ID != f.ID {
				ids = append(ids, existing.ID)
			}
		}
		ids = append(ids, f.ID)
		if err := s.store.ReorderFolders(ids); err != nil {
			return nil, err
		}
		return folderView{ID: f.ID, Name: f.Name, Order: f.Order}, nil
	}
	for _, f := range s.store.Folders() {
		if f.ID == p.ID {
			f.Name = p.Name
			if err := s.store.UpdateFolder(f); err != nil {
				return nil, err
			}
			return folderView{ID: f.ID, Name: f.Name, Order: f.Order}, nil
		}
	}
	return nil, fmt.Errorf("文件夹不存在")
}

func (s *Server) handleDeleteFolder(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if err := s.store.DeleteFolder(p.ID); err != nil {
		return nil, err
	}
	return nil, nil
}
