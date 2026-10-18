// Package store menyimpan setiap event sebagai satu file JSON di
// <data_dir>/events/<id>.json. Tidak ada database: data dimuat ke memori saat
// start dan setiap perubahan ditulis ulang secara atomik.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"transfer-checklist/internal/event"
)

// ErrNotFound dikembalikan bila event tidak ada.
var ErrNotFound = errors.New("event tidak ditemukan")

type slot struct {
	mu sync.Mutex // melindungi ev dan penulisan filenya
	ev *event.Event
}

// Store adalah penyimpanan event berbasis file JSON.
type Store struct {
	dir    string       // <data_dir>/events
	mu     sync.RWMutex // melindungi map events
	events map[string]*slot
}

// Open membuat folder data bila perlu lalu memuat semua event yang ada.
// File yang rusak membuat Open gagal (lebih baik berhenti daripada menimpa data).
func Open(dataDir string) (*Store, error) {
	dir := filepath.Join(dataDir, "events")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("membuat folder data: %w", err)
	}
	s := &Store{dir: dir, events: map[string]*slot{}}

	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("membaca %s: %w", p, err)
		}
		var ev event.Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			return nil, fmt.Errorf("file %s rusak: %w", p, err)
		}
		want := strings.TrimSuffix(filepath.Base(p), ".json")
		if ev.ID != want || !event.ValidID(ev.ID) {
			return nil, fmt.Errorf("file %s: id event %q tidak sesuai nama file", p, ev.ID)
		}
		if ev.Entries == nil {
			ev.Entries = []event.Entry{}
		}
		s.events[ev.ID] = &slot{ev: &ev}
	}
	return s, nil
}

// Dir mengembalikan folder tempat file event disimpan.
func (s *Store) Dir() string { return s.dir }

func clone(ev *event.Event) *event.Event {
	cp := *ev
	cp.Entries = make([]event.Entry, len(ev.Entries))
	copy(cp.Entries, ev.Entries)
	return &cp
}

func (s *Store) slot(id string) *slot {
	if !event.ValidID(id) {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.events[id]
}

// Get mengembalikan salinan event; aman diubah pemanggil.
func (s *Store) Get(id string) (*event.Event, error) {
	sl := s.slot(id)
	if sl == nil {
		return nil, ErrNotFound
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return clone(sl.ev), nil
}

// List mengembalikan salinan semua event, terurut menurut ID.
func (s *Store) List() []*event.Event {
	s.mu.RLock()
	slots := make([]*slot, 0, len(s.events))
	for _, sl := range s.events {
		slots = append(slots, sl)
	}
	s.mu.RUnlock()

	out := make([]*event.Event, 0, len(slots))
	for _, sl := range slots {
		sl.mu.Lock()
		out = append(out, clone(sl.ev))
		sl.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Update menjalankan fn pada salinan event di bawah kunci event tersebut.
// Bila fn mengembalikan error, tidak ada yang ditulis dan error diteruskan.
// Bila fn sukses, file ditulis atomik lalu memori diperbarui.
func (s *Store) Update(id string, fn func(*event.Event) error) error {
	sl := s.slot(id)
	if sl == nil {
		return ErrNotFound
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()

	cp := clone(sl.ev)
	if err := fn(cp); err != nil {
		return err
	}
	cp.ID = id // ID tidak boleh berubah
	if cp.Entries == nil {
		cp.Entries = []event.Entry{}
	}
	if err := s.write(cp); err != nil {
		return err
	}
	sl.ev = cp
	return nil
}

// Create menyimpan event baru. ID dibentuk dari base (slug); bila sudah dipakai
// ditambah akhiran -2, -3, dst. ev.ID diisi dengan ID final.
func (s *Store) Create(base string, ev *event.Event) error {
	if !event.ValidID(base) {
		return fmt.Errorf("slug %q tidak valid", base)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	id := base
	for n := 2; s.events[id] != nil; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	cp := clone(ev)
	cp.ID = id
	if err := s.write(cp); err != nil {
		return err
	}
	s.events[id] = &slot{ev: cp}
	ev.ID = id
	return nil
}

// write menulis event secara atomik: tulis ke .tmp, fsync, simpan versi lama
// sebagai .bak, lalu rename ke nama final. File tidak pernah setengah tertulis.
func (s *Store) write(ev *event.Event) error {
	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	final := filepath.Join(s.dir, ev.ID+".json")
	tmp := final + ".tmp"

	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	if old, err := os.ReadFile(final); err == nil {
		_ = os.WriteFile(final+".bak", old, 0o640) // best effort, satu generasi
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return err
	}
	if d, err := os.Open(s.dir); err == nil { // best effort: fsync direktori
		_ = d.Sync()
		d.Close()
	}
	return nil
}
