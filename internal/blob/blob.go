// Package blob stores raw messages and attachments. The filesystem store is
// for single node deployments; an S3 compatible store (Hetzner Object Storage)
// implements the same interface later.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Store interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	// Delete removes a blob. A missing blob is not an error, so a retried
	// deletion is harmless.
	Delete(ctx context.Context, key string) error
}

type FS struct{ Root string }

func (f FS) path(key string) (string, error) {
	clean := filepath.Clean("/" + key)
	if strings.Contains(key, "..") || clean == "/" {
		return "", errors.New("invalid blob key")
	}
	return filepath.Join(f.Root, clean), nil
}

func (f FS) Put(_ context.Context, key string, data []byte) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("blob mkdir: %w", err)
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return fmt.Errorf("blob write: %w", err)
	}
	return os.Rename(tmp, p)
}

func (f FS) Get(_ context.Context, key string) ([]byte, error) {
	p, err := f.path(key)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

func (f FS) Delete(_ context.Context, key string) error {
	p, err := f.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("blob delete: %w", err)
	}
	// Prune directories left empty (org/yyyy/mm, att/org/message), never the root.
	for dir := filepath.Dir(p); dir != filepath.Clean(f.Root) && strings.HasPrefix(dir, filepath.Clean(f.Root)); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil {
			break // not empty, or already gone
		}
	}
	return nil
}
