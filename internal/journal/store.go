package journal

import (
	"cadenceforge/internal/domain"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
)

type Store struct {
	mu       sync.Mutex
	root     string
	state    domain.State
	lock     *os.File
	poisoned error
}

func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(root, "process.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("data root already in use: %w", err)
	}
	s := &Store{root: root, state: domain.Empty(), lock: lock}
	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err == nil {
		err = json.Unmarshal(data, &s.state)
	} else if os.IsNotExist(err) {
		err = nil
	}
	if err == nil {
		err = domain.ValidateState(s.state)
	}
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("load state: %w", err)
	}
	return s, nil
}
func clone(s domain.State) (domain.State, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return domain.State{}, err
	}
	var out domain.State
	err = json.Unmarshal(b, &out)
	return out, err
}
func (s *Store) Snapshot() (domain.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := clone(s.state)
	if err != nil {
		return domain.State{}, domain.Env("snapshot", "state snapshot failed", err)
	}
	return st, nil
}
func (s *Store) Update(fn func(*domain.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.poisoned != nil {
		return s.poisoned
	}
	next, err := clone(s.state)
	if err != nil {
		return domain.Env("snapshot", "state snapshot failed", err)
	}
	if err = fn(&next); err != nil {
		return err
	}
	if reflect.DeepEqual(next, s.state) {
		return nil
	}
	if err = domain.ValidateState(next); err != nil {
		return domain.Env("commit", "state invariant violated", err)
	}
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return domain.Env("commit", "state encoding failed", err)
	}
	f, err := os.CreateTemp(s.root, ".state-*")
	if err != nil {
		return domain.Env("commit", "state staging failed", err)
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return domain.Env("commit", "state write failed", err)
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return domain.Env("commit", "state sync failed", err)
	}
	if err = f.Close(); err != nil {
		return domain.Env("commit", "state close failed", err)
	}
	if err = os.Rename(name, filepath.Join(s.root, "state.json")); err != nil {
		return domain.Env("commit", "state replace failed", err)
	}
	s.state = next
	d, err := os.Open(s.root)
	if err != nil {
		s.poisoned = domain.Uncertain("commit", "state committed but durability uncertain; restart required", err)
		return s.poisoned
	}
	err = d.Sync()
	d.Close()
	if err != nil {
		s.poisoned = domain.Uncertain("commit", "state committed but durability uncertain; restart required", err)
		return s.poisoned
	}
	return nil
}
func (s *Store) Close() error {
	if s.lock != nil {
		return s.lock.Close()
	}
	return nil
}
