package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"transfer-checklist/internal/event"
)

func newEvent(name string) *event.Event {
	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	return &event.Event{Name: name, StartDate: "2026-10-20", EndDate: "2026-10-31", CreatedAt: now, UpdatedAt: now}
}

func TestCreateGetPersistReload(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ev := newEvent("Arisan")
	if err := s.Create("arisan", ev); err != nil {
		t.Fatal(err)
	}
	if ev.ID != "arisan" {
		t.Fatalf("id = %q", ev.ID)
	}

	// slug bentrok -> akhiran -2, -3
	ev2, ev3 := newEvent("Arisan"), newEvent("Arisan")
	if err := s.Create("arisan", ev2); err != nil || ev2.ID != "arisan-2" {
		t.Fatalf("ev2: %v id=%q", err, ev2.ID)
	}
	if err := s.Create("arisan", ev3); err != nil || ev3.ID != "arisan-3" {
		t.Fatalf("ev3: %v id=%q", err, ev3.ID)
	}

	err = s.Update("arisan", func(e *event.Event) error {
		e.Entries = append(e.Entries, event.Entry{ID: "abc", Name: "Budi", Amount: 150000, TransferDate: "2026-10-21", Status: event.StatusPending})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// file ada dan JSON valid dengan format PRD
	raw, err := os.ReadFile(filepath.Join(dir, "events", "arisan.json"))
	if err != nil {
		t.Fatal(err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(raw, &onDisk); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "name", "description", "start_date", "end_date", "created_at", "updated_at", "entries"} {
		if _, ok := onDisk[k]; !ok {
			t.Errorf("field %q tidak ada di JSON", k)
		}
	}
	entries := onDisk["entries"].([]any)
	first := entries[0].(map[string]any)
	if first["verified_at"] != nil {
		t.Errorf("verified_at harus null, got %v", first["verified_at"])
	}

	// backup dibuat saat menimpa, dan tidak ada sisa .tmp
	if _, err := os.Stat(filepath.Join(dir, "events", "arisan.json.bak")); err != nil {
		t.Errorf(".bak tidak ada: %v", err)
	}
	if tmp, _ := filepath.Glob(filepath.Join(dir, "events", "*.tmp")); len(tmp) != 0 {
		t.Errorf("sisa tmp: %v", tmp)
	}

	// muat ulang dari disk
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s2.Get("arisan")
	if err != nil || len(got.Entries) != 1 || got.Entries[0].Name != "Budi" {
		t.Fatalf("reload: %+v %v", got, err)
	}
	if n := len(s2.List()); n != 3 {
		t.Errorf("List = %d event, want 3", n)
	}
}

func TestGetReturnsCopy(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.Create("x", newEvent("X"))
	_ = s.Update("x", func(e *event.Event) error {
		e.Entries = append(e.Entries, event.Entry{ID: "1", Name: "A"})
		return nil
	})
	a, _ := s.Get("x")
	a.Entries[0].Name = "DIUBAH"
	a.Name = "DIUBAH"
	b, _ := s.Get("x")
	if b.Entries[0].Name != "A" || b.Name != "X" {
		t.Error("Get harus mengembalikan salinan; perubahan luar tidak boleh bocor ke store")
	}
}

func TestUpdateErrorDoesNotWrite(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.Create("x", newEvent("X"))
	boom := errors.New("boom")
	err := s.Update("x", func(e *event.Event) error {
		e.Name = "BERUBAH"
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	got, _ := s.Get("x")
	if got.Name != "X" {
		t.Errorf("Name = %q; perubahan dari fn yang gagal tidak boleh tersimpan", got.Name)
	}
}

func TestNotFoundAndPathTraversal(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	for _, id := range []string{"nope", "../x", "a/b", "", "..", "A"} {
		if _, err := s.Get(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q) err = %v, want ErrNotFound", id, err)
		}
		if err := s.Update(id, func(*event.Event) error { return nil }); !errors.Is(err, ErrNotFound) {
			t.Errorf("Update(%q) err = %v, want ErrNotFound", id, err)
		}
	}
	if err := s.Create("../evil", newEvent("x")); err == nil {
		t.Error("Create dengan slug tidak valid harus gagal")
	}
}

func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	_ = s.Create("x", newEvent("X"))

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := s.Update("x", func(e *event.Event) error {
				e.Entries = append(e.Entries, event.Entry{ID: fmt.Sprintf("%02d", i), Name: "A"})
				return nil
			})
			if err != nil {
				t.Errorf("update %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	got, _ := s.Get("x")
	if len(got.Entries) != n {
		t.Fatalf("memori: %d entri, want %d", len(got.Entries), n)
	}
	s2, err := Open(dir) // file di disk harus valid dan lengkap (AC-08)
	if err != nil {
		t.Fatal(err)
	}
	got2, _ := s2.Get("x")
	if len(got2.Entries) != n {
		t.Fatalf("disk: %d entri, want %d", len(got2.Entries), n)
	}
}

func TestOpenRejectsCorruptAndMismatchedFiles(t *testing.T) {
	dir := t.TempDir()
	events := filepath.Join(dir, "events")
	_ = os.MkdirAll(events, 0o755)

	_ = os.WriteFile(filepath.Join(events, "rusak.json"), []byte("{bukan json"), 0o644)
	if _, err := Open(dir); err == nil {
		t.Error("file JSON rusak harus membuat Open gagal")
	}

	_ = os.WriteFile(filepath.Join(events, "rusak.json"), []byte(`{"id":"lain","entries":[]}`), 0o644)
	if _, err := Open(dir); err == nil {
		t.Error("id tidak sesuai nama file harus membuat Open gagal")
	}

	// file .bak / .tmp diabaikan
	_ = os.WriteFile(filepath.Join(events, "rusak.json"), []byte(`{"id":"rusak","entries":null}`), 0o644)
	_ = os.WriteFile(filepath.Join(events, "x.json.bak"), []byte("sampah"), 0o644)
	_ = os.WriteFile(filepath.Join(events, "y.json.tmp"), []byte("sampah"), 0o644)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ev, _ := s.Get("rusak")
	if ev.Entries == nil {
		t.Error("entries null di file harus dimuat sebagai slice kosong")
	}
}
