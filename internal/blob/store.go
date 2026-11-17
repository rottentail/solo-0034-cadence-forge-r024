package blob

import (
	"cadenceforge/internal/domain"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// SizeError marks an upload outside the accepted byte bound: the client can
// correct it, unlike filesystem failures while staging the bytes.
type SizeError struct{ Limit int64 }

func (e *SizeError) Error() string { return fmt.Sprintf("upload must contain 1..%d bytes", e.Limit) }

type Store struct{ Root string }

func Open(root string) (*Store, error) {
	for _, dir := range []string{"objects", "staging", "renders"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			return nil, err
		}
	}
	return &Store{Root: root}, nil
}
func (s *Store) Object(digest string) (string, error) {
	if !digestPattern.MatchString(digest) {
		return "", domain.Env("object", "invalid content digest", nil)
	}
	return filepath.Join(s.Root, "objects", digest), nil
}
func (s *Store) Receive(r io.Reader, limit int64) (string, int64, error) {
	f, err := os.CreateTemp(filepath.Join(s.Root, "staging"), "upload-*")
	if err != nil {
		return "", 0, domain.Env("receive", "upload staging failed", err)
	}
	defer os.Remove(f.Name())
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, limit+1))
	if err != nil {
		f.Close()
		return "", n, domain.Env("receive", "upload read failed", err)
	}
	if n == 0 || n > limit {
		f.Close()
		return "", n, &SizeError{Limit: limit}
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return "", n, domain.Env("receive", "upload sync failed", err)
	}
	if err = f.Close(); err != nil {
		return "", n, domain.Env("receive", "upload close failed", err)
	}
	digest := hex.EncodeToString(h.Sum(nil))
	dest, _ := s.Object(digest)
	if err = os.Link(f.Name(), dest); err != nil && !os.IsExist(err) {
		return "", n, domain.Env("receive", "object commit failed", err)
	}
	if err = SyncDir(filepath.Dir(dest)); err != nil {
		return "", n, domain.Uncertain("receive", "object linked but durability uncertain", err)
	}
	return digest, n, nil
}
func (s *Store) Staging(id string, attempt int) (string, error) {
	if !idPattern.MatchString(id) {
		return "", domain.Env("staging", "invalid render id", nil)
	}
	dir, err := os.MkdirTemp(filepath.Join(s.Root, "staging"), fmt.Sprintf("render-%s-%d-", id, attempt))
	if err != nil {
		return "", domain.Env("staging", "render staging failed", err)
	}
	return dir, nil
}
func (s *Store) RenderDir(id string, attempt int) (string, error) {
	if !idPattern.MatchString(id) {
		return "", domain.Env("object", "invalid render id", nil)
	}
	return filepath.Join(s.Root, "renders", fmt.Sprintf("%s-%d", id, attempt)), nil
}
func SyncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (s *Store) Promote(stage, id string, attempt int) error {
	entries, err := os.ReadDir(stage)
	if err != nil {
		return domain.Env("promote", "staging read failed", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			return domain.Env("promote", "unexpected output directory", nil)
		}
		f, err := os.OpenFile(filepath.Join(stage, e.Name()), os.O_RDWR, 0600)
		if err != nil {
			return domain.Env("promote", "artifact open failed", err)
		}
		err = f.Sync()
		f.Close()
		if err != nil {
			return domain.Env("promote", "artifact sync failed", err)
		}
	}
	if err = SyncDir(stage); err != nil {
		return domain.Env("promote", "staging sync failed", err)
	}
	dest, err := s.RenderDir(id, attempt)
	if err != nil {
		return err
	}
	if err = os.Rename(stage, dest); err != nil {
		return domain.Env("promote", "artifact rename failed", err)
	}
	if err = SyncDir(filepath.Dir(dest)); err != nil {
		return domain.Uncertain("promote", "artifact promoted but durability uncertain", err)
	}
	return nil
}
