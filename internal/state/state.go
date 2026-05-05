package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type HostState struct {
	Favorite        bool   `json:"favorite,omitempty"`
	ConnectCount    int    `json:"connectCount,omitempty"`
	LastConnectedAt string `json:"lastConnectedAt,omitempty"`
}

type Store struct {
	Hosts       map[string]HostState `json:"hosts"`
	GroupOrder  []string             `json:"groupOrder,omitempty"`
	EmptyGroups []string             `json:"emptyGroups,omitempty"`
}

func NewStore() Store {
	return Store{Hosts: map[string]HostState{}}
}

func DefaultPath() string {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "ssht", "state.json")
	}
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "ssht", "state.json")
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state", "ssht", "state.json")
	}
	return "ssht-state.json"
}

func Load(path string) (Store, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewStore(), nil
	}
	if err != nil {
		return Store{}, err
	}
	store := NewStore()
	if err := json.Unmarshal(content, &store); err != nil {
		return Store{}, fmt.Errorf("load state %s: %w", path, err)
	}
	store.ensure()
	return store, nil
}

func Save(path string, store Store) error {
	store.ensure()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	content, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(path, content, 0o600)
}

func (s *Store) ToggleFavorite(alias string) {
	s.ensure()
	host := s.Hosts[alias]
	host.Favorite = !host.Favorite
	s.Hosts[alias] = host
}

func (s *Store) RecordConnect(alias string, now time.Time) {
	s.ensure()
	host := s.Hosts[alias]
	host.ConnectCount++
	host.LastConnectedAt = now.Format(time.RFC3339)
	s.Hosts[alias] = host
}

func (s *Store) ensure() {
	if s.Hosts == nil {
		s.Hosts = map[string]HostState{}
	}
}
