package web

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"transfer-checklist/internal/event"
	"transfer-checklist/internal/store"
)

type homeEvent struct {
	Event      *event.Event
	Count      int
	ValidCount int
}

type homeView struct{ Events []homeEvent }

// handleHome hanya menampilkan event yang sedang aktif (dibuka dan belum
// ditutup manual). Boleh ada beberapa sekaligus; pengunjung memilih salah satu.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	now, loc := s.now(), s.cfg.Location
	items := []homeEvent{}
	for _, ev := range s.store.List() {
		if !ev.IsActive(now, loc) {
			continue
		}
		sum := ev.Summary()
		items = append(items, homeEvent{Event: ev, Count: sum.Total, ValidCount: sum.Valid})
	}
	// Yang paling cepat berakhir di atas, lalu menurut nama.
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].Event, items[j].Event
		if a.EndDate != b.EndDate {
			return a.EndDate < b.EndDate
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	s.render(w, r, http.StatusOK, "home", "", homeView{Events: items})
}

type entryView struct {
	event.Entry
	Dup bool
}

type eventView struct {
	Event   *event.Event
	State   event.PeriodState
	Summary event.Summary
	Entries []entryView
	Form    event.EntryInput
	Errors  event.FieldErrors
	Today   string
	MinDate string
}

func (s *Server) showEvent(w http.ResponseWriter, r *http.Request, id string, status int, form event.EntryInput, errs event.FieldErrors) {
	ev, err := s.store.Get(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			s.notFound(w, r)
		} else {
			s.serverError(w, r, err)
		}
		return
	}
	now, loc := s.now(), s.cfg.Location
	today := now.In(loc).Format(event.DateLayout)

	minDate := ""
	if start, err := time.Parse(event.DateLayout, ev.StartDate); err == nil {
		minDate = start.AddDate(0, 0, -event.EarlyTransferDays).Format(event.DateLayout)
	}
	if form.TransferDate == "" && errs == nil {
		form.TransferDate = today
	}

	entries := make([]entryView, len(ev.Entries))
	for i, en := range ev.Entries {
		entries[i] = entryView{Entry: en}
	}
	// Terbaru dulu (FR-08).
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].SubmittedAt.After(entries[j].SubmittedAt) })

	if errs == nil {
		errs = event.FieldErrors{}
	}
	s.render(w, r, status, "event", ev.Name, eventView{
		Event: ev, State: ev.State(now, loc), Summary: ev.Summary(), Entries: entries,
		Form: form, Errors: errs, Today: today, MinDate: minDate,
	})
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	s.showEvent(w, r, r.PathValue("id"), http.StatusOK, event.EntryInput{}, nil)
}

var (
	errNotOpen = errors.New("periode tidak dibuka")
	errInvalid = errors.New("input tidak valid")
)

func (s *Server) handleSubmitEntry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.entryLimiter.Allow(s.clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		s.errorPage(w, r, http.StatusTooManyRequests, "Terlalu banyak percobaan", "Tunggu satu menit lalu coba lagi.", "/e/"+id, "Kembali")
		return
	}

	in := event.EntryInput{
		Name:         r.PostFormValue("name"),
		Amount:       r.PostFormValue("amount"),
		TransferDate: r.PostFormValue("transfer_date"),
		Note:         r.PostFormValue("note"),
	}
	now, loc := s.now(), s.cfg.Location

	var (
		fieldErrs  event.FieldErrors
		entry      event.Entry
		state      event.PeriodState
		start, end string
		manual     bool
	)
	// Periode dicek ulang di dalam kunci event, sehingga perubahan tanggal oleh
	// admin di antara dua request tidak bisa terlewat.
	err := s.store.Update(id, func(ev *event.Event) error {
		if st := ev.State(now, loc); st != event.StateOpen {
			state, start, end, manual = st, ev.StartDate, ev.EndDate, ev.ClosedManually()
			return errNotOpen
		}
		en, errs := ev.NewEntry(in, now, loc)
		if len(errs) > 0 {
			fieldErrs = errs
			return errInvalid
		}
		ev.Entries = append(ev.Entries, en)
		ev.UpdatedAt = now.In(loc).Truncate(time.Second)
		entry = en
		return nil
	})

	switch {
	case err == nil:
		s.setFlash(w, fmt.Sprintf("Terima kasih, %s. Transfer %s tanggal %s sudah tercatat dan menunggu verifikasi admin.",
			entry.Name, rupiah(entry.Amount), dateID(entry.TransferDate)))
		http.Redirect(w, r, "/e/"+id+"#entries", http.StatusSeeOther)
	case errors.Is(err, store.ErrNotFound):
		s.notFound(w, r)
	case errors.Is(err, errNotOpen):
		msg := fmt.Sprintf("Periode input sudah ditutup (%s).", dateID(end))
		switch {
		case manual:
			msg = "Event ini sudah ditutup oleh admin, input tidak bisa dikirim lagi."
		case state == event.StateUpcoming:
			msg = fmt.Sprintf("Periode input belum dibuka (mulai %s).", dateID(start))
		}
		s.errorPage(w, r, http.StatusForbidden, "Input ditolak", msg, "/e/"+id, "Kembali")
	case errors.Is(err, errInvalid):
		s.showEvent(w, r, id, http.StatusUnprocessableEntity, in, fieldErrs)
	default:
		s.serverError(w, r, err)
	}
}
