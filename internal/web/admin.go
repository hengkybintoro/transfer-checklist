package web

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"transfer-checklist/internal/auth"
	"transfer-checklist/internal/event"
	"transfer-checklist/internal/store"
)

// ------------------------------------------------------------- login

type loginView struct {
	Next, Username, Error string
}

// safeNext hanya menerima path internal di bawah /admin (cegah open redirect).
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/admin") || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\\r\n") {
		return "/admin"
	}
	if u, err := url.Parse(next); err != nil || u.Host != "" || u.Scheme != "" {
		return "/admin"
	}
	return next
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.adminUser(r); ok {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, "admin_login", "Login Admin", loginView{Next: safeNext(r.URL.Query().Get("next"))})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip := s.clientIP(r)
	view := loginView{Next: safeNext(r.PostFormValue("next")), Username: r.PostFormValue("username")}

	if s.loginLimiter.Blocked(ip) {
		view.Error = "Terlalu banyak percobaan gagal. Coba lagi dalam 15 menit."
		w.Header().Set("Retry-After", "900")
		s.render(w, r, http.StatusTooManyRequests, "admin_login", "Login Admin", view)
		return
	}

	// Kedua perbandingan selalu dievaluasi (waktu konstan, SEC-01).
	userOK := auth.SecureEqual(r.PostFormValue("username"), s.cfg.Admin.Username)
	passOK := auth.SecureEqual(r.PostFormValue("password"), s.cfg.Admin.Password)
	if !(userOK && passOK) {
		s.loginLimiter.Fail(ip)
		log.Printf("LOGIN gagal ip=%s username=%q", ip, truncate(view.Username, 40))
		view.Error = "Username atau password salah."
		s.render(w, r, http.StatusUnauthorized, "admin_login", "Login Admin", view)
		return
	}

	s.loginLimiter.Reset(ip)
	token := s.auth.NewSession(s.cfg.Admin.Username, s.now())
	s.setCookie(w, sessionCookie, token, int(s.auth.TTL().Seconds()))
	log.Printf("LOGIN berhasil user=%s ip=%s", s.cfg.Admin.Username, ip)
	http.Redirect(w, r, view.Next, http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if user, ok := s.adminUser(r); ok {
		log.Printf("LOGOUT user=%s", user)
	}
	s.setCookie(w, sessionCookie, "", -1)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// ------------------------------------------------------------- dashboard

type dashEvent struct {
	Event   *event.Event
	State   event.PeriodState
	Summary event.Summary
}

type dashView struct{ Events []dashEvent }

func (s *Server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	now, loc := s.now(), s.cfg.Location
	events := s.store.List()
	items := make([]dashEvent, 0, len(events))
	for _, ev := range events {
		items = append(items, dashEvent{Event: ev, State: ev.State(now, loc), Summary: ev.Summary()})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Event.StartDate > items[j].Event.StartDate })
	s.render(w, r, http.StatusOK, "admin_dashboard", "Dashboard Admin", dashView{Events: items})
}

// ------------------------------------------------------------- event: buat / ubah

type eventFormView struct {
	Edit     bool
	Action   string
	BackURL  string
	Form     event.EventInput
	Errors   event.FieldErrors
	Timezone string
}

func (s *Server) handleEventNew(w http.ResponseWriter, r *http.Request) {
	today := s.now().In(s.cfg.Location)
	s.render(w, r, http.StatusOK, "admin_event_form", "Event baru", eventFormView{
		Action:   "/admin/events",
		BackURL:  "/admin",
		Timezone: s.cfg.Timezone,
		Form: event.EventInput{
			StartDate: today.Format(event.DateLayout),
			EndDate:   today.AddDate(0, 0, 7).Format(event.DateLayout),
		},
		Errors: event.FieldErrors{},
	})
}

func formEventInput(r *http.Request) event.EventInput {
	return event.EventInput{
		Name:        r.PostFormValue("name"),
		Description: r.PostFormValue("description"),
		StartDate:   r.PostFormValue("start_date"),
		EndDate:     r.PostFormValue("end_date"),
	}
}

func (s *Server) handleEventCreate(w http.ResponseWriter, r *http.Request) {
	in, errs := formEventInput(r).Validate()
	if len(errs) > 0 {
		s.render(w, r, http.StatusUnprocessableEntity, "admin_event_form", "Event baru", eventFormView{
			Action: "/admin/events", BackURL: "/admin", Form: in, Errors: errs, Timezone: s.cfg.Timezone,
		})
		return
	}
	now := s.now().In(s.cfg.Location).Truncate(time.Second)
	ev := &event.Event{
		Name: in.Name, Description: in.Description, StartDate: in.StartDate, EndDate: in.EndDate,
		CreatedAt: now, UpdatedAt: now, Entries: []event.Entry{},
	}
	if err := s.store.Create(event.Slugify(in.Name), ev); err != nil {
		s.serverError(w, r, err)
		return
	}
	user, _ := s.adminUser(r)
	log.Printf("ADMIN %s membuat event id=%s", user, ev.ID)
	s.setFlash(w, fmt.Sprintf("Event “%s” dibuat.", ev.Name))
	http.Redirect(w, r, "/admin/e/"+ev.ID, http.StatusSeeOther)
}

func (s *Server) handleEventEditForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ev, err := s.store.Get(id)
	if err != nil {
		s.notFound(w, r)
		return
	}
	s.render(w, r, http.StatusOK, "admin_event_form", "Ubah event", eventFormView{
		Edit: true, Action: "/admin/e/" + id, BackURL: "/admin/e/" + id, Timezone: s.cfg.Timezone,
		Form:   event.EventInput{Name: ev.Name, Description: ev.Description, StartDate: ev.StartDate, EndDate: ev.EndDate},
		Errors: event.FieldErrors{},
	})
}

func (s *Server) handleEventUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	in, errs := formEventInput(r).Validate()
	if len(errs) > 0 {
		if _, err := s.store.Get(id); err != nil {
			s.notFound(w, r)
			return
		}
		s.render(w, r, http.StatusUnprocessableEntity, "admin_event_form", "Ubah event", eventFormView{
			Edit: true, Action: "/admin/e/" + id, BackURL: "/admin/e/" + id, Form: in, Errors: errs, Timezone: s.cfg.Timezone,
		})
		return
	}
	now := s.now().In(s.cfg.Location).Truncate(time.Second)
	err := s.store.Update(id, func(ev *event.Event) error {
		ev.Name, ev.Description, ev.StartDate, ev.EndDate = in.Name, in.Description, in.StartDate, in.EndDate
		ev.UpdatedAt = now
		return nil
	})
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	user, _ := s.adminUser(r)
	log.Printf("ADMIN %s mengubah event id=%s", user, id)
	s.setFlash(w, "Perubahan disimpan.")
	http.Redirect(w, r, "/admin/e/"+id, http.StatusSeeOther)
}

// ------------------------------------------------------------- tutup / buka event

var (
	errAlreadyClosed = errors.New("event sudah ditutup manual")
	errNotClosed     = errors.New("event tidak sedang ditutup manual")
)

// afterEventAction mengalihkan ke dashboard atau halaman event, sesuai asal form.
func afterEventAction(w http.ResponseWriter, r *http.Request, id string) {
	target := "/admin/e/" + id
	if r.PostFormValue("to") == "dashboard" {
		target = "/admin"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// handleEventClose menutup event secara manual, tanpa menunggu tanggal selesai.
func (s *Server) handleEventClose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, _ := s.adminUser(r)
	now, loc := s.now(), s.cfg.Location

	var name string
	err := s.store.Update(id, func(ev *event.Event) error {
		name = ev.Name
		if ev.ClosedManually() {
			return errAlreadyClosed
		}
		ev.Close(user, now, loc)
		ev.UpdatedAt = now.In(loc).Truncate(time.Second)
		return nil
	})
	switch {
	case err == nil:
		log.Printf("ADMIN %s menutup event=%s", user, id)
		s.setFlash(w, fmt.Sprintf("Event “%s” ditutup. Form input tidak lagi tersedia untuk publik.", name))
		afterEventAction(w, r, id)
	case errors.Is(err, errAlreadyClosed):
		s.setFlash(w, fmt.Sprintf("Event “%s” sudah ditutup.", name))
		afterEventAction(w, r, id)
	case errors.Is(err, store.ErrNotFound):
		s.notFound(w, r)
	default:
		s.serverError(w, r, err)
	}
}

// handleEventReopen membatalkan penutupan manual. Bila rentang tanggal event
// sudah lewat, admin diberi tahu agar mengubah tanggal selesai.
func (s *Server) handleEventReopen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	user, _ := s.adminUser(r)
	now, loc := s.now(), s.cfg.Location

	var name, start, end string
	var state event.PeriodState
	err := s.store.Update(id, func(ev *event.Event) error {
		name, start, end = ev.Name, ev.StartDate, ev.EndDate
		if !ev.ClosedManually() {
			return errNotClosed
		}
		ev.Reopen()
		ev.UpdatedAt = now.In(loc).Truncate(time.Second)
		state = ev.State(now, loc)
		return nil
	})
	switch {
	case err == nil:
		log.Printf("ADMIN %s membuka kembali event=%s", user, id)
		msg := fmt.Sprintf("Event “%s” dibuka kembali.", name)
		switch state {
		case event.StateUpcoming:
			msg += fmt.Sprintf(" Periode baru mulai %s; form input terbuka mulai tanggal itu.", dateID(start))
		case event.StateClosed:
			msg += fmt.Sprintf(" Namun tanggal selesai (%s) sudah lewat, ubah tanggal selesai agar form input terbuka.", dateID(end))
		}
		s.setFlash(w, msg)
		afterEventAction(w, r, id)
	case errors.Is(err, errNotClosed):
		s.setFlash(w, fmt.Sprintf("Event “%s” tidak sedang ditutup manual.", name))
		afterEventAction(w, r, id)
	case errors.Is(err, store.ErrNotFound):
		s.notFound(w, r)
	default:
		s.serverError(w, r, err)
	}
}

// ------------------------------------------------------------- verifikasi

type tab struct {
	Label  string
	Count  int
	Active bool
	URL    string
}

type adminEventView struct {
	Event     *event.Event
	State     event.PeriodState
	Summary   event.Summary
	Entries   []entryView
	Tabs      []tab
	SortURL   string
	SortLabel string
	Back      string // query filter saat ini, dipakai untuk kembali setelah aksi
}

func normFilter(v string) string {
	switch v {
	case "pending", "valid", "invalid":
		return v
	}
	return "all"
}

func normSort(v string) string {
	if v == "submitted" {
		return v
	}
	return "transfer"
}

func eventURL(id, status, sortKey string) string {
	q := url.Values{}
	if status != "all" {
		q.Set("status", status)
	}
	if sortKey != "transfer" {
		q.Set("sort", sortKey)
	}
	u := "/admin/e/" + id
	if enc := q.Encode(); enc != "" {
		u += "?" + enc
	}
	return u
}

func (s *Server) handleAdminEvent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ev, err := s.store.Get(id)
	if err != nil {
		s.notFound(w, r)
		return
	}
	filter := normFilter(r.URL.Query().Get("status"))
	sortKey := normSort(r.URL.Query().Get("sort"))
	sum := ev.Summary()
	dups := ev.Duplicates()

	entries := make([]entryView, 0, len(ev.Entries))
	for _, en := range ev.Entries {
		if filter != "all" && string(en.Status) != filter {
			continue
		}
		entries = append(entries, entryView{Entry: en, Dup: dups[en.ID]})
	}
	if sortKey == "submitted" {
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].SubmittedAt.After(entries[j].SubmittedAt) })
	} else { // urutan yang sama dengan membaca mutasi rekening (FR-25)
		sort.SliceStable(entries, func(i, j int) bool {
			a, b := entries[i], entries[j]
			if a.TransferDate != b.TransferDate {
				return a.TransferDate < b.TransferDate
			}
			return a.SubmittedAt.Before(b.SubmittedAt)
		})
	}

	mk := func(key, label string, count int) tab {
		return tab{Label: label, Count: count, Active: filter == key, URL: eventURL(id, key, sortKey)}
	}
	tabs := []tab{
		mk("all", "Semua", sum.Total),
		mk("pending", "Menunggu", sum.Pending),
		mk("valid", "Valid", sum.Valid),
		mk("invalid", "Tidak valid", sum.Invalid),
	}

	otherSort, sortLabel := "submitted", "Tanggal transfer ↑ (ganti: waktu input terbaru)"
	if sortKey == "submitted" {
		otherSort, sortLabel = "transfer", "Waktu input terbaru (ganti: tanggal transfer)"
	}

	back := url.Values{"status": {filter}, "sort": {sortKey}}.Encode()
	s.render(w, r, http.StatusOK, "admin_event", "Verifikasi · "+ev.Name, adminEventView{
		Event: ev, State: ev.State(s.now(), s.cfg.Location), Summary: sum, Entries: entries,
		Tabs: tabs, SortURL: eventURL(id, filter, otherSort), SortLabel: sortLabel, Back: back,
	})
}

// backURL membangun URL kembali ke halaman verifikasi dengan filter yang sama.
func backURL(id, back, entryID string) string {
	q, _ := url.ParseQuery(back)
	u := eventURL(id, normFilter(q.Get("status")), normSort(q.Get("sort")))
	if entryID != "" {
		u += "#row-" + entryID
	}
	return u
}

func (s *Server) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	id, entryID := r.PathValue("id"), r.PathValue("entryID")
	user, _ := s.adminUser(r)
	st := event.Status(r.PostFormValue("status"))
	note := r.PostFormValue("note")
	now, loc := s.now(), s.cfg.Location

	var oldSt event.Status
	var name string
	var userErr event.UserError
	err := s.store.Update(id, func(ev *event.Event) error {
		i := ev.FindEntry(entryID)
		if i < 0 {
			return errEntryNotFound
		}
		oldSt, name = ev.Entries[i].Status, ev.Entries[i].Name
		if err := ev.Entries[i].SetStatus(st, note, user, now, loc); err != nil {
			return err
		}
		ev.UpdatedAt = now.In(loc).Truncate(time.Second)
		return nil
	})
	switch {
	case err == nil:
		log.Printf("ADMIN %s status event=%s entry=%s %s -> %s", user, id, entryID, oldSt, st)
		s.setFlash(w, fmt.Sprintf("%s → %s.", name, st.Label()))
		http.Redirect(w, r, backURL(id, r.PostFormValue("back"), entryID), http.StatusSeeOther)
	case errors.Is(err, store.ErrNotFound), errors.Is(err, errEntryNotFound):
		s.errorPage(w, r, http.StatusNotFound, "Tidak ditemukan", "Event atau entri tidak ada (mungkin sudah dihapus).", "/admin", "Ke dashboard")
	case errors.As(err, &userErr):
		s.errorPage(w, r, http.StatusUnprocessableEntity, "Perubahan ditolak", userErr.Error()+".", backURL(id, r.PostFormValue("back"), ""), "Kembali")
	default:
		s.serverError(w, r, err)
	}
}

var errEntryNotFound = errors.New("entri tidak ditemukan")

func (s *Server) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	id, entryID := r.PathValue("id"), r.PathValue("entryID")
	user, _ := s.adminUser(r)
	now, loc := s.now(), s.cfg.Location

	var name string
	var amount int64
	err := s.store.Update(id, func(ev *event.Event) error {
		i := ev.FindEntry(entryID)
		if i < 0 {
			return errEntryNotFound
		}
		name, amount = ev.Entries[i].Name, ev.Entries[i].Amount
		ev.Entries = append(ev.Entries[:i], ev.Entries[i+1:]...)
		ev.UpdatedAt = now.In(loc).Truncate(time.Second)
		return nil
	})
	switch {
	case err == nil:
		log.Printf("ADMIN %s hapus event=%s entry=%s name=%q amount=%d", user, id, entryID, name, amount)
		s.setFlash(w, fmt.Sprintf("Entri %s (%s) dihapus.", name, rupiah(amount)))
		http.Redirect(w, r, backURL(id, r.PostFormValue("back"), ""), http.StatusSeeOther)
	case errors.Is(err, store.ErrNotFound), errors.Is(err, errEntryNotFound):
		s.errorPage(w, r, http.StatusNotFound, "Tidak ditemukan", "Event atau entri tidak ada (mungkin sudah dihapus).", "/admin", "Ke dashboard")
	default:
		s.serverError(w, r, err)
	}
}

// ------------------------------------------------------------- CSV

// csvSafe mencegah CSV injection saat file dibuka di spreadsheet.
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ev, err := s.store.Get(id)
	if err != nil {
		s.notFound(w, r)
		return
	}
	sort.SliceStable(ev.Entries, func(i, j int) bool {
		a, b := ev.Entries[i], ev.Entries[j]
		if a.TransferDate != b.TransferDate {
			return a.TransferDate < b.TransferDate
		}
		return a.SubmittedAt.Before(b.SubmittedAt)
	})

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+ev.ID+`.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("\xef\xbb\xbf")) // BOM agar Excel membaca UTF-8
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"tanggal_transfer", "nama", "nominal", "catatan", "waktu_input", "status", "catatan_admin", "diverifikasi_pada", "diverifikasi_oleh"})
	for _, en := range ev.Entries {
		verifiedAt := ""
		if en.VerifiedAt != nil {
			verifiedAt = en.VerifiedAt.Format(time.RFC3339)
		}
		_ = cw.Write([]string{
			en.TransferDate, csvSafe(en.Name), strconv.FormatInt(en.Amount, 10), csvSafe(en.Note),
			en.SubmittedAt.Format(time.RFC3339), en.Status.Label(), csvSafe(en.AdminNote), verifiedAt, csvSafe(en.VerifiedBy),
		})
	}
	cw.Flush()
}
