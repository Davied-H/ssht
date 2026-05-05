package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadMissingFileReturnsEmptyStore(t *testing.T) {
	store, err := Load(filepath.Join(t.TempDir(), "missing", "state.json"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if store.Hosts == nil {
		t.Fatal("Hosts map is nil")
	}
	if len(store.Hosts) != 0 {
		t.Fatalf("Hosts length = %d, want 0", len(store.Hosts))
	}
}

func TestSaveCreatesParentDirectoryAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	store := NewStore()
	store.ToggleFavorite("prod-api")
	store.RecordConnect("prod-api", time.Date(2026, 5, 1, 10, 30, 0, 0, time.FixedZone("CST", 8*60*60)))

	if err := Save(path, store); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	host := loaded.Hosts["prod-api"]
	if !host.Favorite || host.ConnectCount != 1 || host.LastConnectedAt == "" {
		t.Fatalf("host state = %#v, want favorite count and timestamp", host)
	}
}

func TestToggleFavoriteAndRecordConnect(t *testing.T) {
	store := NewStore()
	now := time.Date(2026, 5, 1, 10, 30, 0, 0, time.UTC)

	store.ToggleFavorite("prod-api")
	store.RecordConnect("prod-api", now)
	store.RecordConnect("prod-api", now.Add(time.Minute))

	host := store.Hosts["prod-api"]
	if !host.Favorite {
		t.Fatal("favorite = false, want true")
	}
	if host.ConnectCount != 2 {
		t.Fatalf("connect count = %d, want 2", host.ConnectCount)
	}
	if host.LastConnectedAt != now.Add(time.Minute).Format(time.RFC3339) {
		t.Fatalf("last connected = %q, want latest timestamp", host.LastConnectedAt)
	}

	store.ToggleFavorite("prod-api")
	if store.Hosts["prod-api"].Favorite {
		t.Fatal("favorite = true after second toggle, want false")
	}
}

func TestSaveAndLoadGroupOrderRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewStore()
	store.GroupOrder = []string{"prod", "dev", "staging"}

	if err := Save(path, store); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := loaded.GroupOrder, []string{"prod", "dev", "staging"}; !equalStringSlice(got, want) {
		t.Fatalf("GroupOrder = %v, want %v", got, want)
	}
}

func TestSaveAndLoadEmptyGroupsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := NewStore()
	store.EmptyGroups = []string{"future-east", "lab"}

	if err := Save(path, store); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := loaded.EmptyGroups, []string{"future-east", "lab"}; !equalStringSlice(got, want) {
		t.Fatalf("EmptyGroups = %v, want %v", got, want)
	}
}

func TestLoadOldFormatYieldsNilSlices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"hosts":{"prod-api":{"favorite":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.GroupOrder != nil {
		t.Fatalf("GroupOrder = %v, want nil", loaded.GroupOrder)
	}
	if loaded.EmptyGroups != nil {
		t.Fatalf("EmptyGroups = %v, want nil", loaded.EmptyGroups)
	}
	if !loaded.Hosts["prod-api"].Favorite {
		t.Fatalf("Hosts[prod-api].Favorite = false, want true")
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLoadCorruptJSONReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected corrupt JSON error")
	}
	if !strings.Contains(err.Error(), "state") {
		t.Fatalf("error = %q, want state context", err.Error())
	}
}
