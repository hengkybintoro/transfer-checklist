package event

import (
	"testing"
	"time"
)

var jkt = time.FixedZone("WIB", 7*3600)

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, jkt)
	if err != nil {
		panic(err)
	}
	return t
}

func TestState_Boundaries(t *testing.T) {
	ev := &Event{StartDate: "2026-10-20", EndDate: "2026-10-31"}
	cases := []struct {
		now  string
		want PeriodState
	}{
		{"2026-10-19 23:59:59", StateUpcoming},
		{"2026-10-20 00:00:00", StateOpen},
		{"2026-10-25 12:00:00", StateOpen},
		{"2026-10-31 23:59:59", StateOpen},
		{"2026-11-01 00:00:00", StateClosed},
	}
	for _, c := range cases {
		if got := ev.State(at(c.now), jkt); got != c.want {
			t.Errorf("State(%s) = %v, want %v", c.now, got, c.want)
		}
	}
}

func TestState_SingleDayAndCorrupt(t *testing.T) {
	one := &Event{StartDate: "2026-10-20", EndDate: "2026-10-20"}
	if one.State(at("2026-10-20 23:59:59"), jkt) != StateOpen {
		t.Error("periode satu hari harus terbuka sepanjang hari")
	}
	bad := &Event{StartDate: "x", EndDate: "y"}
	if bad.State(at("2026-10-20 10:00:00"), jkt) != StateClosed {
		t.Error("tanggal rusak harus dianggap ditutup")
	}
}

func TestParseAmount(t *testing.T) {
	ok := map[string]int64{
		"150000": 150000, "150.000": 150000, "150,000": 150000,
		"Rp150.000": 150000, "rp 1.500.000": 1500000, " 1 ": 1, "1000000000": MaxAmount,
	}
	for in, want := range ok {
		got, err := ParseAmount(in)
		if err != nil || got != want {
			t.Errorf("ParseAmount(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "0", "abc", "-5", "1.5", "150.00", "1.000.0", "1000000001", "99999999999999", "12a", "1,5"} {
		if _, err := ParseAmount(in); err == nil {
			t.Errorf("ParseAmount(%q) seharusnya error", in)
		}
	}
}

func TestNewEntry(t *testing.T) {
	ev := &Event{StartDate: "2026-10-20", EndDate: "2026-10-31"}
	now := at("2026-10-25 09:14:03")

	en, errs := ev.NewEntry(EntryInput{Name: "  Budi   Santoso ", Amount: "150.000", TransferDate: "2026-10-24", Note: " dari  istri "}, now, jkt)
	if len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	if en.Name != "Budi Santoso" || en.Amount != 150000 || en.Note != "dari istri" ||
		en.Status != StatusPending || len(en.ID) != 12 || !en.SubmittedAt.Equal(now) {
		t.Errorf("entry tidak sesuai: %+v", en)
	}

	bad := []struct {
		name string
		in   EntryInput
		key  string
	}{
		{"nama kosong", EntryInput{Name: " ", Amount: "1000", TransferDate: "2026-10-24"}, "name"},
		{"nama 1 huruf", EntryInput{Name: "A", Amount: "1000", TransferDate: "2026-10-24"}, "name"},
		{"nominal 0", EntryInput{Name: "Budi", Amount: "0", TransferDate: "2026-10-24"}, "amount"},
		{"nominal teks", EntryInput{Name: "Budi", Amount: "seratus", TransferDate: "2026-10-24"}, "amount"},
		{"tanggal masa depan", EntryInput{Name: "Budi", Amount: "1000", TransferDate: "2026-10-26"}, "transfer_date"},
		{"tanggal rusak", EntryInput{Name: "Budi", Amount: "1000", TransferDate: "24/10/2026"}, "transfer_date"},
		{"terlalu lama sebelum periode", EntryInput{Name: "Budi", Amount: "1000", TransferDate: "2026-09-18"}, "transfer_date"},
	}
	for _, c := range bad {
		if _, errs := ev.NewEntry(c.in, now, jkt); errs[c.key] == "" {
			t.Errorf("%s: harus ada error di %q, got %v", c.name, c.key, errs)
		}
	}

	// tepat 31 hari sebelum start masih boleh (2026-09-19)
	if _, errs := ev.NewEntry(EntryInput{Name: "Budi", Amount: "1000", TransferDate: "2026-09-19"}, now, jkt); len(errs) != 0 {
		t.Errorf("31 hari sebelum start harus boleh: %v", errs)
	}
	// hari ini boleh
	if _, errs := ev.NewEntry(EntryInput{Name: "Budi", Amount: "1000", TransferDate: "2026-10-25"}, now, jkt); len(errs) != 0 {
		t.Errorf("transfer hari ini harus boleh: %v", errs)
	}
}

func TestEventInputValidate(t *testing.T) {
	in, errs := EventInput{Name: "  Arisan  Oktober ", StartDate: "2026-10-20", EndDate: "2026-10-31"}.Validate()
	if len(errs) != 0 || in.Name != "Arisan Oktober" {
		t.Errorf("valid: %+v %v", in, errs)
	}
	if _, errs := (EventInput{Name: "X", StartDate: "2026-10-20", EndDate: "2026-10-31"}).Validate(); errs["name"] == "" {
		t.Error("nama 1 huruf harus ditolak")
	}
	if _, errs := (EventInput{Name: "Arisan", StartDate: "2026-10-31", EndDate: "2026-10-20"}).Validate(); errs["end_date"] == "" {
		t.Error("end < start harus ditolak (BR-03)")
	}
	if _, errs := (EventInput{Name: "Arisan", StartDate: "2026-10-20", EndDate: "2026-10-20"}).Validate(); len(errs) != 0 {
		t.Errorf("end == start harus boleh: %v", errs)
	}
	if _, errs := (EventInput{Name: "Arisan", StartDate: "bukan", EndDate: ""}).Validate(); errs["start_date"] == "" || errs["end_date"] == "" {
		t.Errorf("tanggal rusak harus ditolak: %v", errs)
	}
}

func TestSetStatus(t *testing.T) {
	en := Entry{Status: StatusPending}
	now := at("2026-10-26 10:00:00")

	if err := en.SetStatus(StatusInvalid, "  tidak ada di mutasi ", "admin", now, jkt); err != nil {
		t.Fatal(err)
	}
	if en.Status != StatusInvalid || en.AdminNote != "tidak ada di mutasi" || en.VerifiedBy != "admin" || en.VerifiedAt == nil {
		t.Errorf("invalid: %+v", en)
	}
	if err := en.SetStatus(StatusValid, "catatan diabaikan", "admin", now, jkt); err != nil {
		t.Fatal(err)
	}
	if en.Status != StatusValid || en.AdminNote != "" || en.VerifiedAt == nil {
		t.Errorf("valid harus menghapus catatan: %+v", en)
	}
	if err := en.SetStatus(StatusPending, "", "admin", now, jkt); err != nil {
		t.Fatal(err)
	}
	if en.Status != StatusPending || en.VerifiedAt != nil || en.VerifiedBy != "" {
		t.Errorf("pending harus menghapus jejak verifikasi: %+v", en)
	}
	if err := en.SetStatus("aneh", "", "admin", now, jkt); err == nil {
		t.Error("status tak dikenal harus ditolak")
	}
	long := make([]byte, MaxNoteLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := en.SetStatus(StatusInvalid, string(long), "admin", now, jkt); err == nil {
		t.Error("catatan terlalu panjang harus ditolak")
	}
}

func TestDuplicatesAndSummary(t *testing.T) {
	ev := &Event{Entries: []Entry{
		{ID: "a", Name: "Budi", Amount: 100, TransferDate: "2026-10-21", Status: StatusPending},
		{ID: "b", Name: "BUDI", Amount: 100, TransferDate: "2026-10-21", Status: StatusValid},
		{ID: "c", Name: "Budi", Amount: 200, TransferDate: "2026-10-21", Status: StatusInvalid},
		{ID: "d", Name: "Sari", Amount: 100, TransferDate: "2026-10-21", Status: StatusValid},
	}}
	d := ev.Duplicates()
	if !d["a"] || !d["b"] || d["c"] || d["d"] {
		t.Errorf("duplicates = %v", d)
	}
	s := ev.Summary()
	if s.Total != 4 || s.Pending != 1 || s.Valid != 2 || s.Invalid != 1 || s.ValidSum != 200 || s.TotalSum != 500 {
		t.Errorf("summary = %+v", s)
	}
	if ev.FindEntry("c") != 2 || ev.FindEntry("zzz") != -1 {
		t.Error("FindEntry salah")
	}
}

func TestSlugAndID(t *testing.T) {
	cases := map[string]string{
		"Arisan Oktober 2026":  "arisan-oktober-2026",
		"  --Iuran / Reuni!! ": "iuran-reuni",
		"日本語":                  "event",
		"":                     "event",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want || !ValidID(got) {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	long := Slugify("a very long event name that keeps going and going and going and going on")
	if len(long) > maxSlugLen || !ValidID(long) {
		t.Errorf("slug terlalu panjang: %q", long)
	}
	for _, bad := range []string{"", "../etc", "a/b", "A", "a_b", "-a", "a-", "a--b", ".."} {
		if ValidID(bad) {
			t.Errorf("ValidID(%q) seharusnya false", bad)
		}
	}
}

func TestManualCloseAndReopen(t *testing.T) {
	ev := &Event{StartDate: "2026-10-20", EndDate: "2026-10-31"}
	mid := at("2026-10-25 12:00:00")

	if !ev.IsActive(mid, jkt) || ev.ClosedManually() {
		t.Fatal("event di tengah periode harus aktif")
	}

	ev.Close("admin", mid, jkt)
	if !ev.ClosedManually() || ev.ClosedBy != "admin" || ev.ClosedAt == nil || !ev.ClosedAt.Equal(mid) {
		t.Fatalf("Close tidak mencatat penutupan: %+v", ev)
	}
	if ev.State(mid, jkt) != StateClosed || ev.IsActive(mid, jkt) {
		t.Error("event yang ditutup manual harus Closed meski tanggal belum selesai")
	}
	// penutupan manual juga menutup event yang belum dimulai
	if ev.State(at("2026-10-10 09:00:00"), jkt) != StateClosed {
		t.Error("penutupan manual harus mengalahkan state upcoming")
	}

	ev.Reopen()
	if ev.ClosedManually() || ev.ClosedBy != "" || ev.ClosedAt != nil {
		t.Fatalf("Reopen harus menghapus jejak penutupan: %+v", ev)
	}
	if !ev.IsActive(mid, jkt) {
		t.Error("setelah dibuka kembali (masih dalam periode) harus aktif lagi")
	}
	// dibuka kembali setelah tanggal selesai: tetap tertutup karena tanggal
	if ev.IsActive(at("2026-11-02 08:00:00"), jkt) {
		t.Error("reopen tidak boleh mengabaikan tanggal selesai")
	}
}
