// Package event berisi model Event/Entry beserta seluruh aturan bisnis
// (periode input, validasi, transisi status). Paket ini tidak tahu apa pun
// tentang HTTP maupun penyimpanan.
package event

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// DateLayout adalah format tanggal yang dipakai di JSON dan form (YYYY-MM-DD).
const DateLayout = "2006-01-02"

// Batas-batas validasi (lihat PRD §6.3).
const (
	MaxAmount         int64 = 1_000_000_000
	MinNameLen              = 2
	MaxNameLen              = 100
	MaxNoteLen              = 200
	MaxEventNameLen         = 100
	MaxDescriptionLen       = 300
	EarlyTransferDays       = 31 // tanggal transfer boleh maks. 31 hari sebelum start_date
	maxIDLen                = 60
	maxSlugLen              = 50
)

// Status adalah status verifikasi sebuah entri.
type Status string

const (
	StatusPending Status = "pending"
	StatusValid   Status = "valid"
	StatusInvalid Status = "invalid"
)

// Known memeriksa apakah status dikenali.
func (s Status) Known() bool {
	return s == StatusPending || s == StatusValid || s == StatusInvalid
}

// Label adalah teks status untuk ditampilkan ke pengguna.
func (s Status) Label() string {
	switch s {
	case StatusValid:
		return "Valid"
	case StatusInvalid:
		return "Tidak Valid"
	default:
		return "Menunggu"
	}
}

// Entry adalah satu catatan transfer.
type Entry struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Amount       int64      `json:"amount"`
	TransferDate string     `json:"transfer_date"`
	Note         string     `json:"note"`
	SubmittedAt  time.Time  `json:"submitted_at"`
	Status       Status     `json:"status"`
	AdminNote    string     `json:"admin_note"`
	VerifiedAt   *time.Time `json:"verified_at"`
	VerifiedBy   string     `json:"verified_by"`
}

// Event adalah satu periode pencatatan; disimpan sebagai satu file JSON.
type Event struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	StartDate   string    `json:"start_date"`
	EndDate     string    `json:"end_date"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// ClosedAt != nil berarti event ditutup manual oleh admin (mengalahkan
	// rentang tanggal). Dibuka kembali dengan Reopen.
	ClosedAt *time.Time `json:"closed_at"`
	ClosedBy string     `json:"closed_by"`
	Entries  []Entry    `json:"entries"`
}

// ---------------------------------------------------------------- periode

// PeriodState adalah posisi "sekarang" terhadap rentang tanggal event.
type PeriodState int

const (
	StateUpcoming PeriodState = iota // belum dibuka
	StateOpen                        // dibuka
	StateClosed                      // ditutup
)

// Label adalah teks status periode.
func (p PeriodState) Label() string {
	switch p {
	case StateUpcoming:
		return "Belum dibuka"
	case StateOpen:
		return "Dibuka"
	default:
		return "Ditutup"
	}
}

// Key adalah penanda singkat untuk CSS/logika template.
func (p PeriodState) Key() string {
	switch p {
	case StateUpcoming:
		return "upcoming"
	case StateOpen:
		return "open"
	default:
		return "closed"
	}
}

// ClosedManually true bila admin menutup event ini secara manual.
func (e *Event) ClosedManually() bool { return e.ClosedAt != nil }

// Close menutup event secara manual, tanpa menunggu tanggal selesai (BR-11).
func (e *Event) Close(by string, now time.Time, loc *time.Location) {
	t := now.In(loc).Truncate(time.Second)
	e.ClosedAt, e.ClosedBy = &t, by
}

// Reopen membatalkan penutupan manual. Status akhirnya tetap mengikuti
// rentang tanggal (bisa saja sudah lewat).
func (e *Event) Reopen() { e.ClosedAt, e.ClosedBy = nil, "" }

// IsActive true bila event sedang menerima input: tidak ditutup manual dan
// now berada dalam rentang tanggal. Hanya event aktif yang tampil di beranda publik.
func (e *Event) IsActive(now time.Time, loc *time.Location) bool {
	return e.State(now, loc) == StateOpen
}

// State menentukan apakah periode input sedang dibuka pada waktu now.
// Penutupan manual selalu menang. Selain itu periode inklusif: mulai
// start_date 00:00:00 hingga end_date 23:59:59 di zona waktu loc (BR-02).
// Tanggal yang rusak dianggap ditutup.
func (e *Event) State(now time.Time, loc *time.Location) PeriodState {
	if e.ClosedAt != nil {
		return StateClosed
	}
	start, err1 := time.ParseInLocation(DateLayout, e.StartDate, loc)
	end, err2 := time.ParseInLocation(DateLayout, e.EndDate, loc)
	if err1 != nil || err2 != nil {
		return StateClosed
	}
	endExclusive := end.AddDate(0, 0, 1)
	switch {
	case now.Before(start):
		return StateUpcoming
	case !now.Before(endExclusive):
		return StateClosed
	default:
		return StateOpen
	}
}

// ---------------------------------------------------------------- validasi

// FieldErrors memetakan nama field form ke pesan kesalahan.
type FieldErrors map[string]string

// ValidID memeriksa slug event (juga nama file, jadi harus aman dari path traversal).
func ValidID(id string) bool {
	return len(id) > 0 && len(id) <= maxIDLen && idPattern.MatchString(id)
}

var idPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Slugify membuat slug dasar dari nama event.
func Slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > maxSlugLen {
		s = strings.Trim(s[:maxSlugLen], "-")
	}
	if s == "" {
		s = "event"
	}
	return s
}

// NewID membuat ID acak 12 karakter hex untuk entri.
func NewID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand gagal: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// NormalizeText merapikan spasi berlebih.
func NormalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

var amountPattern = regexp.MustCompile(`^(\d+|\d{1,3}([.,]\d{3})+)$`)

// ParseAmount menerima "150000", "150.000", "150,000", dan awalan "Rp".
func ParseAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.EqualFold(s[:2], "rp") {
		s = strings.TrimSpace(s[2:])
	}
	s = strings.ReplaceAll(s, " ", "")
	if !amountPattern.MatchString(s) {
		return 0, fmt.Errorf("nominal harus berupa angka bulat, contoh: 150000 atau 150.000")
	}
	s = strings.NewReplacer(".", "", ",", "").Replace(s)
	if len(s) > 12 {
		return 0, fmt.Errorf("nominal terlalu besar")
	}
	var n int64
	for _, c := range s {
		n = n*10 + int64(c-'0')
	}
	if n < 1 {
		return 0, fmt.Errorf("nominal harus lebih dari 0")
	}
	if n > MaxAmount {
		return 0, fmt.Errorf("nominal maksimal Rp1.000.000.000")
	}
	return n, nil
}

// EntryInput adalah data mentah dari form pengirim.
type EntryInput struct {
	Name         string
	Amount       string
	TransferDate string
	Note         string
}

// NewEntry memvalidasi input pengirim terhadap event ini lalu membuat Entry
// berstatus pending. now menjadi submitted_at. Pengecekan periode dilakukan
// pemanggil (lihat State).
func (e *Event) NewEntry(in EntryInput, now time.Time, loc *time.Location) (Entry, FieldErrors) {
	errs := FieldErrors{}
	en := Entry{
		ID:          NewID(),
		SubmittedAt: now.In(loc).Truncate(time.Second),
		Status:      StatusPending,
	}

	en.Name = NormalizeText(in.Name)
	if n := utf8.RuneCountInString(en.Name); n < MinNameLen || n > MaxNameLen {
		errs["name"] = fmt.Sprintf("Nama harus 2–%d karakter.", MaxNameLen)
	}

	amount, err := ParseAmount(in.Amount)
	if err != nil {
		errs["amount"] = capitalize(err.Error()) + "."
	}
	en.Amount = amount

	date := strings.TrimSpace(in.TransferDate)
	if td, err := time.Parse(DateLayout, date); err != nil {
		errs["transfer_date"] = "Tanggal transfer tidak valid."
	} else {
		today := now.In(loc).Format(DateLayout)
		switch {
		case date > today:
			errs["transfer_date"] = "Tanggal transfer tidak boleh di masa depan."
		default:
			if start, err := time.Parse(DateLayout, e.StartDate); err == nil {
				earliest := start.AddDate(0, 0, -EarlyTransferDays)
				if td.Before(earliest) {
					errs["transfer_date"] = fmt.Sprintf("Tanggal transfer terlalu lama (maksimal %d hari sebelum periode dibuka).", EarlyTransferDays)
				}
			}
		}
		en.TransferDate = td.Format(DateLayout)
	}

	en.Note = NormalizeText(in.Note)
	if utf8.RuneCountInString(en.Note) > MaxNoteLen {
		errs["note"] = fmt.Sprintf("Catatan maksimal %d karakter.", MaxNoteLen)
	}

	if len(errs) > 0 {
		return Entry{}, errs
	}
	return en, nil
}

// EventInput adalah data mentah dari form admin untuk membuat/ubah event.
type EventInput struct {
	Name        string
	Description string
	StartDate   string
	EndDate     string
}

// Validate memvalidasi dan merapikan input event (BR-03).
func (in EventInput) Validate() (EventInput, FieldErrors) {
	errs := FieldErrors{}
	out := EventInput{
		Name:        NormalizeText(in.Name),
		Description: NormalizeText(in.Description),
		StartDate:   strings.TrimSpace(in.StartDate),
		EndDate:     strings.TrimSpace(in.EndDate),
	}
	if n := utf8.RuneCountInString(out.Name); n < 2 || n > MaxEventNameLen {
		errs["name"] = fmt.Sprintf("Nama event harus 2–%d karakter.", MaxEventNameLen)
	}
	if utf8.RuneCountInString(out.Description) > MaxDescriptionLen {
		errs["description"] = fmt.Sprintf("Deskripsi maksimal %d karakter.", MaxDescriptionLen)
	}
	s, errS := time.Parse(DateLayout, out.StartDate)
	e, errE := time.Parse(DateLayout, out.EndDate)
	if errS != nil {
		errs["start_date"] = "Tanggal mulai tidak valid."
	}
	if errE != nil {
		errs["end_date"] = "Tanggal selesai tidak valid."
	}
	if errS == nil && errE == nil && e.Before(s) {
		errs["end_date"] = "Tanggal selesai tidak boleh sebelum tanggal mulai."
	}
	return out, errs
}

// UserError adalah kesalahan validasi yang aman ditampilkan ke pengguna.
type UserError string

func (e UserError) Error() string { return string(e) }

// SetStatus mengubah status verifikasi (BR-08). Catatan admin hanya disimpan
// untuk status invalid. Status pending menghapus jejak verifikasi.
func (en *Entry) SetStatus(st Status, note, by string, now time.Time, loc *time.Location) error {
	if !st.Known() {
		return UserError(fmt.Sprintf("Status %q tidak dikenal", st))
	}
	note = NormalizeText(note)
	if utf8.RuneCountInString(note) > MaxNoteLen {
		return UserError(fmt.Sprintf("Catatan admin maksimal %d karakter", MaxNoteLen))
	}
	en.Status = st
	switch st {
	case StatusPending:
		en.AdminNote, en.VerifiedAt, en.VerifiedBy = "", nil, ""
	case StatusValid:
		t := now.In(loc).Truncate(time.Second)
		en.AdminNote, en.VerifiedAt, en.VerifiedBy = "", &t, by
	case StatusInvalid:
		t := now.In(loc).Truncate(time.Second)
		en.AdminNote, en.VerifiedAt, en.VerifiedBy = note, &t, by
	}
	return nil
}

// ---------------------------------------------------------------- bantu

// Summary adalah rekap jumlah dan nominal per status.
type Summary struct {
	Total, Pending, Valid, Invalid             int
	TotalSum, PendingSum, ValidSum, InvalidSum int64
}

// Summary menghitung rekap seluruh entri.
func (e *Event) Summary() Summary {
	var s Summary
	for _, en := range e.Entries {
		s.Total++
		s.TotalSum += en.Amount
		switch en.Status {
		case StatusValid:
			s.Valid++
			s.ValidSum += en.Amount
		case StatusInvalid:
			s.Invalid++
			s.InvalidSum += en.Amount
		default:
			s.Pending++
			s.PendingSum += en.Amount
		}
	}
	return s
}

// FindEntry mengembalikan indeks entri dengan ID tertentu, atau -1.
func (e *Event) FindEntry(id string) int {
	for i := range e.Entries {
		if e.Entries[i].ID == id {
			return i
		}
	}
	return -1
}

// Duplicates mengembalikan himpunan ID entri yang kemungkinan duplikat:
// nama (tanpa beda huruf besar/kecil), nominal, dan tanggal transfer sama (FR-28).
func (e *Event) Duplicates() map[string]bool {
	groups := map[string][]string{}
	for _, en := range e.Entries {
		key := fmt.Sprintf("%s|%d|%s", strings.ToLower(en.Name), en.Amount, en.TransferDate)
		groups[key] = append(groups[key], en.ID)
	}
	dups := map[string]bool{}
	for _, ids := range groups {
		if len(ids) > 1 {
			for _, id := range ids {
				dups[id] = true
			}
		}
	}
	return dups
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
