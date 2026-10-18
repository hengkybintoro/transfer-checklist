// Package web berisi handler HTTP, middleware, dan rendering template.
package web

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"transfer-checklist/internal/auth"
	"transfer-checklist/internal/config"
	"transfer-checklist/internal/event"
	"transfer-checklist/internal/store"
	"transfer-checklist/web"
)

const (
	sessionCookie = "tc_session"
	csrfCookie    = "tc_csrf"
	flashCookie   = "tc_flash"
	maxBodyBytes  = 16 << 10 // 16 KB (SEC-05)
)

// Server menggabungkan konfigurasi, store, dan template.
type Server struct {
	cfg   *config.Config
	store *store.Store
	auth  *auth.Manager
	tpl   map[string]*template.Template

	loginLimiter *auth.Limiter // kegagalan login per IP
	entryLimiter *auth.Limiter // input entri per IP

	now func() time.Time // dapat diganti saat test
	mux *http.ServeMux
}

// New membuat Server dan mendaftarkan seluruh route.
func New(cfg *config.Config, st *store.Store) (*Server, error) {
	s := &Server{
		cfg:          cfg,
		store:        st,
		auth:         auth.NewManager(cfg.SessionSecret, cfg.SessionTTL()),
		loginLimiter: auth.NewLimiter(5, 15*time.Minute),
		entryLimiter: auth.NewLimiter(10, time.Minute),
		now:          time.Now,
		mux:          http.NewServeMux(),
	}
	if err := s.loadTemplates(); err != nil {
		return nil, err
	}
	s.routes()
	return s, nil
}

// SetClock mengganti sumber waktu server (untuk test).
func (s *Server) SetClock(now func() time.Time) {
	s.now = now
	s.loginLimiter.SetClock(now)
	s.entryLimiter.SetClock(now)
}

// Handler mengembalikan http.Handler lengkap dengan middleware.
func (s *Server) Handler() http.Handler {
	return s.logRequests(s.secureHeaders(s.protect(s.mux)))
}

func (s *Server) routes() {
	staticFS, _ := fs.Sub(webfs.FS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})

	// publik
	s.mux.HandleFunc("GET /{$}", s.handleHome)
	s.mux.HandleFunc("GET /e/{id}", s.handleEvent)
	s.mux.HandleFunc("POST /e/{id}/entries", s.handleSubmitEntry)

	// admin
	s.mux.HandleFunc("GET /admin/login", s.handleLoginForm)
	s.mux.HandleFunc("POST /admin/login", s.handleLogin)
	s.mux.HandleFunc("POST /admin/logout", s.handleLogout)
	s.mux.HandleFunc("GET /admin", s.requireAdmin(s.handleAdminDashboard))
	s.mux.HandleFunc("GET /admin/events/new", s.requireAdmin(s.handleEventNew))
	s.mux.HandleFunc("POST /admin/events", s.requireAdmin(s.handleEventCreate))
	s.mux.HandleFunc("GET /admin/e/{id}", s.requireAdmin(s.handleAdminEvent))
	s.mux.HandleFunc("GET /admin/e/{id}/edit", s.requireAdmin(s.handleEventEditForm))
	s.mux.HandleFunc("POST /admin/e/{id}", s.requireAdmin(s.handleEventUpdate))
	s.mux.HandleFunc("POST /admin/e/{id}/close", s.requireAdmin(s.handleEventClose))
	s.mux.HandleFunc("POST /admin/e/{id}/reopen", s.requireAdmin(s.handleEventReopen))
	s.mux.HandleFunc("POST /admin/e/{id}/entries/{entryID}/status", s.requireAdmin(s.handleSetStatus))
	s.mux.HandleFunc("POST /admin/e/{id}/entries/{entryID}/delete", s.requireAdmin(s.handleDeleteEntry))
	s.mux.HandleFunc("GET /admin/e/{id}/export.csv", s.requireAdmin(s.handleExportCSV))

	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.errorPage(w, r, http.StatusNotFound, "Halaman tidak ditemukan", "Alamat yang Anda buka tidak ada.", "/", "Ke beranda")
	})
}

// ------------------------------------------------------------- middleware

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/static/") || r.URL.Path == "/healthz" {
			return
		}
		log.Printf("%s %s %d %s ip=%s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond), s.clientIP(r))
	})
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'")
		next.ServeHTTP(w, r)
	})
}

// protect membatasi ukuran body dan memverifikasi CSRF pada semua request POST (SEC-03).
func (s *Server) protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
			if err := r.ParseForm(); err != nil {
				s.errorPage(w, r, http.StatusRequestEntityTooLarge, "Permintaan ditolak", "Data yang dikirim terlalu besar atau rusak.", "/", "Ke beranda")
				return
			}
			c, err := r.Cookie(csrfCookie)
			if err != nil || !s.auth.VerifyCSRF(c.Value, r.PostFormValue("csrf_token")) {
				s.errorPage(w, r, http.StatusForbidden, "Sesi form kedaluwarsa", "Muat ulang halaman sebelumnya lalu coba lagi.", "/", "Ke beranda")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireAdmin mengalihkan ke halaman login bila belum login.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := s.adminUser(r); !ok {
			target := "/admin/login"
			if r.Method == http.MethodGet {
				target += "?next=" + url.QueryEscape(r.URL.RequestURI())
			}
			http.Redirect(w, r, target, http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func (s *Server) adminUser(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return "", false
	}
	return s.auth.VerifySession(c.Value, s.now())
}

// clientIP menentukan IP klien; header proxy hanya dipercaya bila trust_proxy aktif.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if v := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); v != "" {
			return v
		}
		if v := r.Header.Get("X-Forwarded-For"); v != "" {
			first, _, _ := strings.Cut(v, ",")
			if first = strings.TrimSpace(first); first != "" {
				return first
			}
		}
		if v := strings.TrimSpace(r.Header.Get("X-Real-IP")); v != "" {
			return v
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ------------------------------------------------------------- cookie

func (s *Server) setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.cfg.Secure(),
		SameSite: http.SameSiteLaxMode,
	})
}

// csrfToken memastikan cookie CSRF ada lalu mengembalikan token untuk form.
func (s *Server) csrfToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookie); err == nil && len(c.Value) == 32 {
		return s.auth.CSRFToken(c.Value)
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand gagal: " + err.Error())
	}
	val := hex.EncodeToString(b[:])
	s.setCookie(w, csrfCookie, val, 30*24*3600)
	return s.auth.CSRFToken(val)
}

func (s *Server) setFlash(w http.ResponseWriter, msg string) {
	s.setCookie(w, flashCookie, url.QueryEscape(msg), 60)
}

func (s *Server) takeFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(flashCookie)
	if err != nil {
		return ""
	}
	s.setCookie(w, flashCookie, "", -1)
	msg, err := url.QueryUnescape(c.Value)
	if err != nil {
		return ""
	}
	return msg
}

// ------------------------------------------------------------- rendering

type pageData struct {
	Title   string
	IsAdmin bool
	CSRF    string
	Flash   string
	Data    any
}

type errorData struct {
	Heading, Message, BackURL, BackLabel string
}

func (s *Server) loadTemplates() error {
	funcs := template.FuncMap{
		"rupiah":     rupiah,
		"dateID":     dateID,
		"timeID":     timeID,
		"statusIcon": statusIcon,
	}
	pages := []string{"home", "event", "error", "admin_login", "admin_dashboard", "admin_event_form", "admin_event"}
	s.tpl = make(map[string]*template.Template, len(pages))
	for _, p := range pages {
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(webfs.FS, "templates/layout.html", "templates/"+p+".html")
		if err != nil {
			return fmt.Errorf("template %s: %w", p, err)
		}
		s.tpl[p] = t
	}
	return nil
}

// render menulis halaman ke buffer dulu agar kegagalan template tidak menghasilkan output setengah jadi.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page, title string, data any) {
	_, isAdmin := s.adminUser(r)
	pd := pageData{
		Title:   title,
		IsAdmin: isAdmin,
		CSRF:    s.csrfToken(w, r),
		Flash:   s.takeFlash(w, r),
		Data:    data,
	}
	var buf bytes.Buffer
	if err := s.tpl[page].ExecuteTemplate(&buf, "layout.html", pd); err != nil {
		log.Printf("ERROR render %s: %v", page, err)
		http.Error(w, "Terjadi kesalahan pada server.", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func (s *Server) errorPage(w http.ResponseWriter, r *http.Request, status int, heading, msg, back, backLabel string) {
	s.render(w, r, status, "error", heading, errorData{Heading: heading, Message: msg, BackURL: back, BackLabel: backLabel})
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("ERROR %s %s: %v", r.Method, r.URL.Path, err)
	s.errorPage(w, r, http.StatusInternalServerError, "Terjadi kesalahan", "Data belum tersimpan. Silakan coba lagi.", "/", "Ke beranda")
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusNotFound, "Tidak ditemukan", "Event yang Anda cari tidak ada.", "/", "Ke beranda")
}

// ------------------------------------------------------------- format

var monthsID = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func rupiah(n int64) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-Rp" + b.String()
	}
	return "Rp" + b.String()
}

// dateID mengubah "2026-10-21" menjadi "21 Okt 2026".
func dateID(s string) string {
	t, err := time.Parse(event.DateLayout, s)
	if err != nil {
		return s
	}
	return fmt.Sprintf("%d %s %d", t.Day(), monthsID[t.Month()-1], t.Year())
}

// timeID mengubah waktu menjadi "21 Okt 2026 09:14" (menerima time.Time atau *time.Time).
func timeID(v any) string {
	var t time.Time
	switch x := v.(type) {
	case time.Time:
		t = x
	case *time.Time:
		if x == nil {
			return ""
		}
		t = *x
	default:
		return ""
	}
	return fmt.Sprintf("%d %s %d %02d:%02d", t.Day(), monthsID[t.Month()-1], t.Year(), t.Hour(), t.Minute())
}

func statusIcon(st event.Status) string {
	switch st {
	case event.StatusValid:
		return "✓"
	case event.StatusInvalid:
		return "✗"
	default:
		return "⏳"
	}
}
