package server

import (
	"encoding/json"
	"fmt"
	"strings"

	sshclient "sshclient/ssh"
	"sshclient/storage"
)

// keyView is the managed-key shape sent to the browser. It carries only the
// public half and metadata; the private material never leaves the backend.
type keyView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Comment       string `json:"comment"`
	PublicKey     string `json:"publicKey"`
	Fingerprint   string `json:"fingerprint"`
	KeyType       string `json:"keyType"`
	HasPassphrase bool   `json:"hasPassphrase"`
}

func toKeyView(k *storage.SSHKey) keyView {
	return keyView{
		ID:            k.ID,
		Name:          k.Name,
		Comment:       k.Comment,
		PublicKey:     k.PublicKey,
		Fingerprint:   k.Fingerprint,
		KeyType:       k.KeyType,
		HasPassphrase: k.EncryptedPassphrase != "",
	}
}

func (s *Server) handleListKeys(c *wsClient, params json.RawMessage) (interface{}, error) {
	ks := s.store.Keys()
	out := make([]keyView, 0, len(ks))
	for _, k := range ks {
		out = append(out, toKeyView(k))
	}
	return out, nil
}

// saveKeyParams submits a user key. privateKey is only required when creating a
// key or replacing its material; leaving it empty on edit keeps the stored key.
type saveKeyParams struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Comment    string `json:"comment"`
	PrivateKey string `json:"privateKey"`
	Passphrase string `json:"passphrase"`
}

func (s *Server) handleSaveKey(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p saveKeyParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return nil, fmt.Errorf("请输入密钥名称")
	}

	if p.ID == "" {
		if strings.TrimSpace(p.PrivateKey) == "" {
			return nil, fmt.Errorf("请提交私钥内容")
		}
		k, err := buildKey(p)
		if err != nil {
			return nil, err
		}
		k.ID = storage.NewID()
		if err := s.store.AddKey(k); err != nil {
			return nil, err
		}
		s.NotifyAll(UIMsg{Type: UIKeysChanged})
		return toKeyView(k), nil
	}

	existing, err := s.findKey(p.ID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.PrivateKey) == "" && p.Passphrase != "" {
		return nil, fmt.Errorf("修改口令需同时重新提交私钥")
	}
	existing.Name = p.Name
	existing.Comment = p.Comment
	// An empty private key means "keep the stored key material"; only metadata
	// changes. Resubmitting a key replaces the material and passphrase.
	if strings.TrimSpace(p.PrivateKey) != "" {
		replacement, err := buildKey(p)
		if err != nil {
			return nil, err
		}
		existing.EncryptedPrivateKey = replacement.EncryptedPrivateKey
		existing.EncryptedPassphrase = replacement.EncryptedPassphrase
		existing.PublicKey = replacement.PublicKey
		existing.Fingerprint = replacement.Fingerprint
		existing.KeyType = replacement.KeyType
	}
	if err := s.store.UpdateKey(existing); err != nil {
		return nil, err
	}
	s.NotifyAll(UIMsg{Type: UIKeysChanged})
	return toKeyView(existing), nil
}

// buildKey validates and encrypts the submitted key material, returning an
// SSHKey populated with the derived public metadata (no ID set).
func buildKey(p saveKeyParams) (*storage.SSHKey, error) {
	info, err := sshclient.ParseKey([]byte(p.PrivateKey), p.Passphrase)
	if err != nil {
		if sshclient.PassphraseRequired(err) {
			return nil, fmt.Errorf("该私钥已加密，请填写口令")
		}
		return nil, fmt.Errorf("解析私钥失败：%w", err)
	}
	enc, err := storage.EncryptSecret(p.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("保存私钥失败：%w", err)
	}
	var encPass string
	if p.Passphrase != "" {
		if encPass, err = storage.EncryptSecret(p.Passphrase); err != nil {
			return nil, fmt.Errorf("保存口令失败：%w", err)
		}
	}
	return &storage.SSHKey{
		Name:                p.Name,
		Comment:             p.Comment,
		EncryptedPrivateKey: enc,
		EncryptedPassphrase: encPass,
		PublicKey:           info.PublicKey,
		Fingerprint:         info.Fingerprint,
		KeyType:             info.KeyType,
	}, nil
}

func (s *Server) handleDeleteKey(c *wsClient, params json.RawMessage) (interface{}, error) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	if err := s.store.DeleteKey(p.ID); err != nil {
		return nil, err
	}
	s.NotifyAll(UIMsg{Type: UIKeysChanged})
	return nil, nil
}

func (s *Server) findKey(id string) (*storage.SSHKey, error) {
	for _, k := range s.store.Keys() {
		if k.ID == id {
			return k, nil
		}
	}
	return nil, fmt.Errorf("密钥不存在")
}

// allKeyMaterials decrypts every managed key so an ad-hoc (quick connect)
// session can try them, the way the ssh client tries every identity.
func (s *Server) allKeyMaterials() []sshclient.KeyMaterial {
	var out []sshclient.KeyMaterial
	for _, k := range s.store.Keys() {
		pem, passphrase, err := s.store.KeyMaterial(k.ID)
		if err != nil || pem == "" {
			continue
		}
		out = append(out, sshclient.KeyMaterial{PEM: pem, Passphrase: passphrase})
	}
	return out
}
