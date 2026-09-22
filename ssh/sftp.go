package ssh

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// SFTPResult reports the outcome of a single file transfer.
type SFTPResult struct {
	Source      string
	Destination string
	Bytes       int64
	Err         error
}

// TransferKind selects upload or download direction.
type TransferKind int

const (
	Upload TransferKind = iota
	Download
)

// TransferFile copies a single file over SFTP between local and remote.
func TransferFile(client *ssh.Client, kind TransferKind, localPath, remotePath string) SFTPResult {
	res := SFTPResult{Source: localPath, Destination: remotePath}
	c, err := sftp.NewClient(client)
	if err != nil {
		res.Err = fmt.Errorf("open sftp: %w", err)
		return res
	}
	defer c.Close()

	if kind == Upload {
		local, err := os.Open(localPath)
		if err != nil {
			res.Err = fmt.Errorf("open local: %w", err)
			return res
		}
		defer local.Close()
		remote, err := c.Create(remotePath)
		if err != nil {
			res.Err = fmt.Errorf("create remote: %w", err)
			return res
		}
		n, err := io.Copy(remote, local)
		closeErr := remote.Close()
		res.Bytes = n
		if err != nil {
			res.Err = fmt.Errorf("upload: %w", err)
		} else if closeErr != nil {
			res.Err = fmt.Errorf("close remote: %w", closeErr)
		}
		return res
	}

	remote, err := c.Open(remotePath)
	if err != nil {
		res.Err = fmt.Errorf("open remote: %w", err)
		return res
	}
	defer remote.Close()
	local, err := os.Create(localPath)
	if err != nil {
		res.Err = fmt.Errorf("create local: %w", err)
		return res
	}
	n, err := io.Copy(local, remote)
	closeErr := local.Close()
	res.Bytes = n
	if err != nil {
		res.Err = fmt.Errorf("download: %w", err)
	} else if closeErr != nil {
		res.Err = fmt.Errorf("close local: %w", closeErr)
	}
	return res
}

// EnsureRemoteDir creates a directory on the remote host (like mkdir -p).
func EnsureRemoteDir(client *ssh.Client, remoteDir string) error {
	c, err := sftp.NewClient(client)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.MkdirAll(remoteDir)
}

// RemotePathJoin joins remote segments the same way filepath.Join does locally.
func RemotePathJoin(elem ...string) string {
	return path.Join(elem...)
}
