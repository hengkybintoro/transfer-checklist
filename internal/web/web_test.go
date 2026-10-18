package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"transfer-checklist/internal/config"
	"transfer-checklist/internal/store"
)

var jkt = time.FixedZone("WIB", 7*3600)

type env struct {
	t       *testing.T
	ts      *httptest.Server
	dataDir string
	clock   atomic.Int64 // unix nano
}

func (e *env) setNow(s string) {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, jkt)
	if err != nil {
		e.t.Fatal(err)
	}
	e.clock.Store(t.UnixNano())
}

func newEnv(t *testing.T) *env {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Parse([]byte(`{
		"data_dir": `+jsonStr(dir)+`,
		"admin": {"username": "admin", "password": "rahasia-banget"},
		"session_secret": "0123456789abcdef0123456789abcdef"
	}`), func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(cfg, st)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, dataDir: dir}
	e.setNow("2026-10-25 12:00:00")
	srv.SetClock(func() time.Time { return time.Unix(0, e.clock.Load()).In(jkt) })
	e.ts = httptest.NewServer(srv.Handler())
	t.Cleanup(e.ts.Close)
	return e
}

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }

type client struct {
	e *env
	c *http.Client
}

func (e *env) newClient() *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, c: &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (c *client) do(req *http.Request) (int, string, http.Header) {
	c.e.t.Helper()
	resp, err := c.c.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

func (c *client) get(path string) (int, string, http.Header) {
	c.e.t.Helper()
	req, _ := http.NewRequest("GET", c.e.ts.URL+path, nil)
	return c.do(req)
}

var csrfRe = regexp.MustCompile(`name="csrf_token" value="([0-9a-f]+)"`)

// token mengambil token CSRF dari sebuah halaman (sekaligus menanam cookie CSRF).
func (c *client) token(path string) string {
	c.e.t.Helper()
	_, body, _ := c.get(path)
	m := csrfRe.FindStringSubmatch(body)
	if m == nil {
		c.e.t.Fatalf("tidak ada csrf_token di %s", path)
	}
	return m[1]
}

// post mengirim form dengan token CSRF yang diambil dari tokenPage.
func (c *client) post(path, tokenPage string, v url.Values) (int, string, http.Header) {
	c.e.t.Helper()
	if v == nil {
		v = url.Values{}
	}
	if tokenPage != "" {
		v.Set("csrf_token", c.token(tokenPage))
	}
	req, _ := http.NewRequest("POST", c.e.ts.URL+path, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

func (e *env) adminClient() *client {
	e.t.Helper()
	c := e.newClient()
	code, _, h := c.post("/admin/login", "/admin/login", url.Values{"username": {"admin"}, "password": {"rahasia-banget"}})
	if code != http.StatusSeeOther || h.Get("Location") != "/admin" {
		e.t.Fatalf("login admin gagal: %d %s", code, h.Get("Location"))
	}
	return c
}

func (e *env) createEvent(admin *client, name, start, end string) string {
	e.t.Helper()
	code, _, h := admin.post("/admin/events", "/admin/events/new", url.Values{
		"name": {name}, "description": {"Transfer ke BCA 123 a.n. Strapa"}, "start_date": {start}, "end_date": {end},
	})
	if code != http.StatusSeeOther {
		e.t.Fatalf("buat event: %d", code)
	}
	return strings.TrimPrefix(h.Get("Location"), "/admin/e/")
}

func entryForm(name, amount, date string) url.Values {
	return url.Values{"name": {name}, "amount": {amount}, "transfer_date": {date}, "note": {"dari istri"}}
}

func firstEntryID(t *testing.T, body string) string {
	t.Helper()
	m := regexp.MustCompile(`id="row-([0-9a-f]{12})"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("tidak ada baris entri di halaman admin")
	}
	return m[1]
}

// ---------------------------------------------------------------- tes

func TestAdminCreatesEventFileAC05(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan Oktober 2026", "2026-10-20", "2026-10-31")
	if id != "arisan-oktober-2026" {
		t.Fatalf("id = %q", id)
	}
	raw, err := os.ReadFile(filepath.Join(e.dataDir, "events", id+".json"))
	if err != nil {
		t.Fatalf("file event tidak dibuat: %v", err)
	}
	var ev map[string]any
	if err := json.Unmarshal(raw, &ev); err != nil || ev["start_date"] != "2026-10-20" || ev["end_date"] != "2026-10-31" {
		t.Errorf("isi file salah: %v %s", err, raw)
	}

	// event kedua dengan nama sama -> slug unik
	if id2 := e.createEvent(admin, "Arisan Oktober 2026", "2026-11-01", "2026-11-05"); id2 != "arisan-oktober-2026-2" {
		t.Errorf("id2 = %q", id2)
	}

	// event tampil di beranda publik
	code, body, _ := e.newClient().get("/")
	if code != 200 || !strings.Contains(body, "Arisan Oktober 2026") || !strings.Contains(body, "Dibuka") {
		t.Errorf("beranda: %d", code)
	}
}

func TestEventValidationOnCreate(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	code, body, _ := admin.post("/admin/events", "/admin/events/new", url.Values{
		"name": {"A"}, "start_date": {"2026-10-31"}, "end_date": {"2026-10-20"},
	})
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "Nama event harus") || !strings.Contains(body, "tidak boleh sebelum") {
		t.Errorf("code=%d", code)
	}
	if files, _ := filepath.Glob(filepath.Join(e.dataDir, "events", "*.json")); len(files) != 0 {
		t.Error("event tidak valid tidak boleh tersimpan")
	}
}

func TestSubmitAndVerifyFlowAC01_06_07(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	page := "/e/" + id

	pub := e.newClient()

	// AC-01: kirim entri -> redirect, tampil Menunggu, submitted_at otomatis
	code, _, h := pub.post(page+"/entries", page, entryForm("Budi Santoso", "150.000", "2026-10-24"))
	if code != http.StatusSeeOther || !strings.HasPrefix(h.Get("Location"), page) {
		t.Fatalf("submit: %d %s", code, h.Get("Location"))
	}
	_, body, _ := pub.get(page)
	for _, want := range []string{"Terima kasih, Budi Santoso", "Budi Santoso", "Rp150.000", "24 Okt 2026", "25 Okt 2026 12:00", "Menunggu"} {
		if !strings.Contains(body, want) {
			t.Errorf("halaman publik tidak memuat %q", want)
		}
	}
	if strings.Contains(body, "dari istri") {
		t.Error("catatan pengirim tidak boleh tampil di halaman publik")
	}
	// flash hanya sekali
	if _, body2, _ := pub.get(page); strings.Contains(body2, "Terima kasih, Budi") {
		t.Error("flash harus hilang setelah dibaca")
	}

	// halaman admin menampilkan catatan pengirim
	adminPage := "/admin/e/" + id
	_, abody, _ := admin.get(adminPage)
	if !strings.Contains(abody, "dari istri") {
		t.Error("admin harus melihat catatan pengirim")
	}
	entryID := firstEntryID(t, abody)
	statusURL := adminPage + "/entries/" + entryID + "/status"

	// AC-06: tandai Valid
	e.setNow("2026-10-25 20:30:00")
	code, _, h = admin.post(statusURL, adminPage, url.Values{"status": {"valid"}, "back": {"status=all&sort=transfer"}})
	if code != http.StatusSeeOther || !strings.HasSuffix(h.Get("Location"), "#row-"+entryID) {
		t.Fatalf("set valid: %d %s", code, h.Get("Location"))
	}
	_, body, _ = pub.get(page)
	if !strings.Contains(body, "badge-valid") || !strings.Contains(body, "✓ Valid") {
		t.Error("badge Valid tidak tampil di halaman publik")
	}
	raw, _ := os.ReadFile(filepath.Join(e.dataDir, "events", id+".json"))
	var disk struct {
		Entries []struct {
			Status     string  `json:"status"`
			VerifiedAt *string `json:"verified_at"`
			VerifiedBy string  `json:"verified_by"`
		} `json:"entries"`
	}
	_ = json.Unmarshal(raw, &disk)
	if len(disk.Entries) != 1 || disk.Entries[0].Status != "valid" || disk.Entries[0].VerifiedBy != "admin" ||
		disk.Entries[0].VerifiedAt == nil || !strings.HasPrefix(*disk.Entries[0].VerifiedAt, "2026-10-25T20:30:00+07:00") {
		t.Errorf("verifikasi tidak tercatat di JSON: %+v", disk.Entries)
	}

	// AC-07: Tidak Valid + catatan tampil publik
	admin.post(statusURL, adminPage, url.Values{"status": {"invalid"}, "note": {"tidak ada di mutasi 24/10"}})
	_, body, _ = pub.get(page)
	if !strings.Contains(body, "badge-invalid") || !strings.Contains(body, "tidak ada di mutasi 24/10") {
		t.Error("status Tidak Valid + catatan harus tampil publik")
	}

	// reset ke menunggu
	admin.post(statusURL, adminPage, url.Values{"status": {"pending"}})
	_, body, _ = pub.get(page)
	if !strings.Contains(body, "badge-pending") || strings.Contains(body, "tidak ada di mutasi") {
		t.Error("reset ke Menunggu harus menghapus catatan")
	}

	// status aneh ditolak
	code, _, _ = admin.post(statusURL, adminPage, url.Values{"status": {"hacked"}})
	if code != http.StatusUnprocessableEntity {
		t.Errorf("status tak dikenal: %d", code)
	}
}

func TestPeriodEnforcedOnServerAC02(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	page := "/e/" + id
	pub := e.newClient()

	cases := []struct {
		now, want string
		form      bool
	}{
		{"2026-10-19 23:59:59", "belum dibuka", false},
		{"2026-11-01 00:00:00", "sudah ditutup", false},
		{"2026-10-20 00:00:00", "", true},
		{"2026-10-31 23:59:59", "", true},
	}
	for _, c := range cases {
		e.setNow(c.now)
		_, body, _ := pub.get(page)
		if got := strings.Contains(body, `name="amount"`); got != c.form {
			t.Errorf("%s: form tampil = %v, want %v", c.now, got, c.form)
		}
		code, pbody, _ := pub.post(page+"/entries", "/admin/login", entryForm("Budi", "1000", "2026-10-19"))
		if c.form {
			if code != http.StatusSeeOther {
				t.Errorf("%s: code = %d, want 303", c.now, code)
			}
			continue
		}
		if code != http.StatusForbidden || !strings.Contains(pbody, c.want) {
			t.Errorf("%s: code = %d (want 403 berisi %q)", c.now, code, c.want)
		}
		if !strings.Contains(body, c.want) {
			t.Errorf("%s: halaman tidak memuat pesan %q", c.now, c.want)
		}
	}
	// hanya dua entri yang masuk (batas atas & bawah periode)
	raw, _ := os.ReadFile(filepath.Join(e.dataDir, "events", id+".json"))
	if n := strings.Count(string(raw), `"submitted_at"`); n != 2 {
		t.Errorf("entri tersimpan = %d, want 2", n)
	}
}

func TestInputValidationKeepsValuesAC03(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	page := "/e/" + id
	pub := e.newClient()

	bad := []struct {
		name string
		form url.Values
		msg  string
	}{
		{"nominal 0", entryForm("Budi Santoso", "0", "2026-10-24"), "lebih dari 0"},
		{"nominal teks", entryForm("Budi Santoso", "banyak", "2026-10-24"), "angka bulat"},
		{"nama kosong", entryForm("", "1000", "2026-10-24"), "Nama harus"},
		{"tanggal masa depan", entryForm("Budi Santoso", "1000", "2026-10-26"), "masa depan"},
	}
	for _, c := range bad {
		code, body, _ := pub.post(page+"/entries", page, c.form)
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, c.msg) {
			t.Errorf("%s: code=%d, pesan %q tidak ada", c.name, code, c.msg)
		}
	}
	// nilai yang sudah diisi tidak hilang
	_, body, _ := pub.post(page+"/entries", page, entryForm("Budi Santoso", "0", "2026-10-24"))
	if !strings.Contains(body, `value="Budi Santoso"`) || !strings.Contains(body, `value="2026-10-24"`) {
		t.Error("nilai form hilang setelah error")
	}
	raw, _ := os.ReadFile(filepath.Join(e.dataDir, "events", id+".json"))
	if strings.Contains(string(raw), "submitted_at") {
		t.Error("input tidak valid tidak boleh tersimpan")
	}
}

func TestAdminAuthAC04(t *testing.T) {
	e := newEnv(t)
	c := e.newClient()

	for _, p := range []string{"/admin", "/admin/events/new", "/admin/e/x", "/admin/e/x/export.csv"} {
		code, _, h := c.get(p)
		if code != http.StatusSeeOther || !strings.HasPrefix(h.Get("Location"), "/admin/login") {
			t.Errorf("GET %s tanpa login: %d %s", p, code, h.Get("Location"))
		}
	}
	// POST admin tanpa login juga ditolak (CSRF valid, tapi bukan admin)
	code, _, h := c.post("/admin/events", "/admin/login", url.Values{"name": {"x"}})
	if code != http.StatusSeeOther || !strings.HasPrefix(h.Get("Location"), "/admin/login") {
		t.Errorf("POST admin tanpa login: %d %s", code, h.Get("Location"))
	}

	// kredensial salah
	code, body, _ := c.post("/admin/login", "/admin/login", url.Values{"username": {"admin"}, "password": {"salah"}})
	if code != http.StatusUnauthorized || !strings.Contains(body, "salah") {
		t.Errorf("login salah: %d", code)
	}
	// benar + next
	code, _, h = c.post("/admin/login", "/admin/login", url.Values{"username": {"admin"}, "password": {"rahasia-banget"}, "next": {"/admin/events/new"}})
	if code != http.StatusSeeOther || h.Get("Location") != "/admin/events/new" {
		t.Errorf("login benar: %d %s", code, h.Get("Location"))
	}
	if code, _, _ := c.get("/admin"); code != 200 {
		t.Errorf("setelah login: %d", code)
	}
	// logout
	if code, _, _ := c.post("/admin/logout", "/admin", nil); code != http.StatusSeeOther {
		t.Errorf("logout: %d", code)
	}
	if code, _, _ := c.get("/admin"); code != http.StatusSeeOther {
		t.Errorf("setelah logout: %d", code)
	}

	// open redirect ditolak
	for _, next := range []string{"//evil.com", "https://evil.com", "/admin/../x\\y", "/lain"} {
		if got := safeNext(next); got == next && next != "/admin/../x\\y" {
			t.Errorf("safeNext(%q) lolos", next)
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t)
	c := e.newClient()
	for i := 0; i < 5; i++ {
		if code, _, _ := c.post("/admin/login", "/admin/login", url.Values{"username": {"admin"}, "password": {"salah"}}); code != http.StatusUnauthorized {
			t.Fatalf("percobaan %d: %d", i+1, code)
		}
	}
	// percobaan ke-6 diblokir meski password benar
	code, _, _ := c.post("/admin/login", "/admin/login", url.Values{"username": {"admin"}, "password": {"rahasia-banget"}})
	if code != http.StatusTooManyRequests {
		t.Errorf("code = %d, want 429", code)
	}
	// setelah 15 menit boleh lagi
	e.setNow("2026-10-25 12:16:00")
	code, _, _ = c.post("/admin/login", "/admin/login", url.Values{"username": {"admin"}, "password": {"rahasia-banget"}})
	if code != http.StatusSeeOther {
		t.Errorf("setelah jendela: %d", code)
	}
}

func TestEntryRateLimit(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	page := "/e/" + id
	pub := e.newClient()
	tok := pub.token(page)
	var last int
	for i := 0; i < 11; i++ {
		v := entryForm("Budi", "1000", "2026-10-24")
		v.Set("csrf_token", tok)
		req, _ := http.NewRequest("POST", e.ts.URL+page+"/entries", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		last, _, _ = pub.do(req)
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("entri ke-11 dalam semenit: %d, want 429", last)
	}
}

func TestCSRF(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	page := "/e/" + id

	pub := e.newClient()
	pub.get(page) // cookie CSRF ada
	// tanpa token
	code, _, _ := pub.post(page+"/entries", "", entryForm("Budi", "1000", "2026-10-24"))
	if code != http.StatusForbidden {
		t.Errorf("tanpa token: %d", code)
	}
	// token palsu
	v := entryForm("Budi", "1000", "2026-10-24")
	v.Set("csrf_token", strings.Repeat("0", 64))
	if code, _, _ := pub.post(page+"/entries", "", v); code != http.StatusForbidden {
		t.Errorf("token palsu: %d", code)
	}
	// token dari klien lain (cookie berbeda)
	other := e.newClient()
	v.Set("csrf_token", other.token(page))
	if code, _, _ := pub.post(page+"/entries", "", v); code != http.StatusForbidden {
		t.Errorf("token klien lain: %d", code)
	}
	// admin: aksi tanpa token ditolak
	if code, _, _ := admin.post("/admin/logout", "", nil); code != http.StatusForbidden {
		t.Errorf("logout tanpa token: %d", code)
	}
	if code, _, _ := admin.get("/admin"); code != 200 {
		t.Errorf("sesi harus tetap hidup: %d", code)
	}
}

func TestDuplicateFlagDeleteAndFilters(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	page, adminPage := "/e/"+id, "/admin/e/"+id
	pub := e.newClient()

	pub.post(page+"/entries", page, entryForm("Budi", "100000", "2026-10-22"))
	pub.post(page+"/entries", page, entryForm("budi", "100.000", "2026-10-22"))
	pub.post(page+"/entries", page, entryForm("Sari", "50000", "2026-10-21"))

	_, body, _ := admin.get(adminPage)
	if n := strings.Count(body, "Kemungkinan duplikat"); n != 2 {
		t.Errorf("penanda duplikat = %d, want 2", n)
	}
	// urut tanggal transfer naik: Sari (21) sebelum Budi (22)
	if strings.Index(body, "Sari") > strings.Index(body, "Budi") {
		t.Error("urutan default harus tanggal transfer naik")
	}

	// filter status
	_, body, _ = admin.get(adminPage + "?status=valid")
	if strings.Contains(body, "Sari") || !strings.Contains(body, "Tidak ada entri untuk filter ini") {
		t.Error("filter valid harus kosong")
	}

	// hapus entri Sari
	_, body, _ = admin.get(adminPage)
	re := regexp.MustCompile(`id="row-([0-9a-f]{12})"[\s\S]*?Sari`)
	sariID := re.FindStringSubmatch(body)[1]
	code, _, _ := admin.post(adminPage+"/entries/"+sariID+"/delete", adminPage, nil)
	if code != http.StatusSeeOther {
		t.Fatalf("hapus: %d", code)
	}
	pub.get(page) // konsumsi flash "Terima kasih, Sari…" milik pub
	_, body, _ = pub.get(page)
	if strings.Contains(body, "Sari") || !strings.Contains(body, "2 entri") {
		t.Error("entri Sari harus terhapus")
	}
	// hapus lagi -> 404
	if code, _, _ := admin.post(adminPage+"/entries/"+sariID+"/delete", adminPage, nil); code != http.StatusNotFound {
		t.Errorf("hapus ulang: %d", code)
	}
}

func TestEditEventDates(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-01", "2026-10-10") // sudah ditutup
	page := "/e/" + id
	pub := e.newClient()
	if code, _, _ := pub.post(page+"/entries", "/admin/login", entryForm("Budi", "1000", "2026-10-05")); code != http.StatusForbidden {
		t.Fatalf("harus ditutup: %d", code)
	}
	code, _, _ := admin.post("/admin/e/"+id, "/admin/e/"+id+"/edit", url.Values{
		"name": {"Arisan (diperpanjang)"}, "start_date": {"2026-10-01"}, "end_date": {"2026-10-31"},
	})
	if code != http.StatusSeeOther {
		t.Fatalf("edit: %d", code)
	}
	if code, _, _ := pub.post(page+"/entries", page, entryForm("Budi", "1000", "2026-10-05")); code != http.StatusSeeOther {
		t.Errorf("setelah diperpanjang harus bisa input: %d", code)
	}
	_, body, _ := pub.get(page)
	if !strings.Contains(body, "Arisan (diperpanjang)") {
		t.Error("nama baru tidak tampil")
	}
}

func TestCSVExport(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	page := "/e/" + id
	pub := e.newClient()
	pub.post(page+"/entries", page, entryForm("=HYPERLINK(\"http://x\")", "150000", "2026-10-24"))

	code, body, h := admin.get("/admin/e/" + id + "/export.csv")
	if code != 200 || !strings.HasPrefix(h.Get("Content-Type"), "text/csv") ||
		!strings.Contains(h.Get("Content-Disposition"), id+".csv") {
		t.Fatalf("export: %d %v", code, h)
	}
	if !strings.HasPrefix(body, "\xef\xbb\xbftanggal_transfer,nama,nominal") {
		t.Errorf("header CSV salah: %q", body[:min(len(body), 60)])
	}
	if !strings.Contains(body, `'=HYPERLINK`) || !strings.Contains(body, ",150000,") || !strings.Contains(body, "Menunggu") {
		t.Errorf("isi CSV salah: %s", body)
	}
}

func TestXSSEscapedAndHeaders(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	page := "/e/" + id
	pub := e.newClient()
	pub.post(page+"/entries", page, entryForm(`<script>alert(1)</script>`, "1000", "2026-10-24"))

	code, body, h := pub.get(page)
	if code != 200 || strings.Contains(body, "<script>alert(1)") {
		t.Error("nama harus di-escape")
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Error("nama ter-escape tidak ditemukan")
	}
	for k, v := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY", "Referrer-Policy": "same-origin"} {
		if h.Get(k) != v {
			t.Errorf("header %s = %q", k, h.Get(k))
		}
	}
	if !strings.Contains(h.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Error("CSP tidak ada")
	}
}

func TestMiscRoutes(t *testing.T) {
	e := newEnv(t)
	c := e.newClient()
	if code, body, _ := c.get("/healthz"); code != 200 || strings.TrimSpace(body) != "ok" {
		t.Errorf("healthz: %d %q", code, body)
	}
	if code, _, _ := c.get("/e/tidak-ada"); code != 404 {
		t.Errorf("event tak ada: %d", code)
	}
	if code, _, _ := c.get("/e/../../etc/passwd"); code == 200 {
		t.Error("path traversal lolos")
	}
	if code, _, _ := c.get("/tidak-ada"); code != 404 {
		t.Errorf("halaman tak ada: %d", code)
	}
	if code, body, _ := c.get("/static/style.css"); code != 200 || !strings.Contains(body, "--primary") {
		t.Errorf("static css: %d", code)
	}
	if code, _, _ := c.get("/static/app.js"); code != 200 {
		t.Errorf("static js: %d", code)
	}
}

func TestTrustProxyIP(t *testing.T) {
	cfg := &config.Config{}
	s := &Server{cfg: cfg}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	if got := s.clientIP(req); got != "127.0.0.1" {
		t.Errorf("tanpa trust_proxy header harus diabaikan, got %q", got)
	}
	cfg.TrustProxy = true
	if got := s.clientIP(req); got != "203.0.113.9" {
		t.Errorf("XFF: %q", got)
	}
	req.Header.Set("CF-Connecting-IP", "198.51.100.7")
	if got := s.clientIP(req); got != "198.51.100.7" {
		t.Errorf("CF: %q", got)
	}
}

func TestFormatters(t *testing.T) {
	for in, want := range map[int64]string{0: "Rp0", 999: "Rp999", 1000: "Rp1.000", 150000: "Rp150.000", 1000000000: "Rp1.000.000.000"} {
		if got := rupiah(in); got != want {
			t.Errorf("rupiah(%d) = %q, want %q", in, got, want)
		}
	}
	if got := dateID("2026-10-05"); got != "5 Okt 2026" {
		t.Errorf("dateID = %q", got)
	}
	if got := dateID("rusak"); got != "rusak" {
		t.Errorf("dateID rusak = %q", got)
	}
	ts := time.Date(2026, 5, 3, 9, 4, 0, 0, jkt)
	if got := timeID(ts); got != "3 Mei 2026 09:04" {
		t.Errorf("timeID = %q", got)
	}
	if got := timeID(&ts); got != "3 Mei 2026 09:04" {
		t.Errorf("timeID ptr = %q", got)
	}
	var nilT *time.Time
	if got := timeID(nilT); got != "" {
		t.Errorf("timeID nil = %q", got)
	}
}

// ---------------------------------------------------------------- tutup manual & event aktif

func TestHomeListsOnlyActiveEvents(t *testing.T) {
	e := newEnv(t) // sekarang: 2026-10-25 12:00
	admin := e.adminClient()

	e.createEvent(admin, "Arisan Aktif A", "2026-10-20", "2026-10-31")
	e.createEvent(admin, "Iuran Aktif B", "2026-10-25", "2026-11-05")
	e.createEvent(admin, "Event Akan Datang", "2026-11-10", "2026-11-20")
	e.createEvent(admin, "Event Sudah Lewat", "2026-09-01", "2026-09-10")
	manualID := e.createEvent(admin, "Event Ditutup Manual", "2026-10-01", "2026-12-31")
	if code, _, _ := admin.post("/admin/e/"+manualID+"/close", "/admin/e/"+manualID, nil); code != http.StatusSeeOther {
		t.Fatalf("close: %d", code)
	}

	code, body, _ := e.newClient().get("/")
	if code != 200 {
		t.Fatal(code)
	}
	// beberapa event aktif sekaligus, urut paling cepat berakhir dulu
	a, b := strings.Index(body, "Arisan Aktif A"), strings.Index(body, "Iuran Aktif B")
	if a < 0 || b < 0 || a > b {
		t.Errorf("dua event aktif harus tampil dan terurut menurut tanggal selesai (a=%d b=%d)", a, b)
	}
	for _, hidden := range []string{"Event Akan Datang", "Event Sudah Lewat", "Event Ditutup Manual"} {
		if strings.Contains(body, hidden) {
			t.Errorf("%q tidak boleh tampil di beranda publik", hidden)
		}
	}
	if !strings.Contains(body, "Input dibuka sampai 31 Okt 2026") || !strings.Contains(body, "Transfer ke BCA 123") {
		t.Error("kartu event harus memuat tanggal berakhir dan deskripsi")
	}

	// admin tetap melihat semuanya
	_, abody, _ := admin.get("/admin")
	for _, name := range []string{"Arisan Aktif A", "Iuran Aktif B", "Event Akan Datang", "Event Sudah Lewat", "Event Ditutup Manual"} {
		if !strings.Contains(abody, name) {
			t.Errorf("dashboard admin harus memuat %q", name)
		}
	}
}

func TestHomeEmptyWhenNoActiveEvent(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	e.createEvent(admin, "Sudah Lewat", "2026-09-01", "2026-09-10")
	_, body, _ := e.newClient().get("/")
	if !strings.Contains(body, "Belum ada event yang aktif") || strings.Contains(body, "Sudah Lewat") {
		t.Error("tanpa event aktif, beranda harus menampilkan pesan kosong")
	}
}

func TestCloseEventManuallyAndReopen(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	page, adminPage := "/e/"+id, "/admin/e/"+id
	pub := e.newClient()

	// sebelum ditutup: bisa input & tampil di beranda
	if code, _, _ := pub.post(page+"/entries", page, entryForm("Budi", "1000", "2026-10-24")); code != http.StatusSeeOther {
		t.Fatalf("input sebelum tutup: %d", code)
	}
	if _, body, _ := pub.get("/"); !strings.Contains(body, "Arisan") {
		t.Fatal("event aktif harus ada di beranda")
	}
	_, abody, _ := admin.get(adminPage)
	if !strings.Contains(abody, "Tutup event") || strings.Contains(abody, "Buka kembali") {
		t.Error("event aktif: harus ada tombol Tutup, bukan Buka kembali")
	}

	// tutup manual
	e.setNow("2026-10-25 15:45:10")
	code, _, h := admin.post(adminPage+"/close", adminPage, nil)
	if code != http.StatusSeeOther || h.Get("Location") != adminPage {
		t.Fatalf("close: %d %s", code, h.Get("Location"))
	}
	_, abody, _ = admin.get(adminPage)
	if !strings.Contains(abody, "ditutup oleh admin") && !strings.Contains(abody, "ditutup manual oleh admin") {
		t.Error("halaman admin harus menandai penutupan manual")
	}
	if !strings.Contains(abody, "Buka kembali") || strings.Contains(abody, "Tutup event") {
		t.Error("event ditutup manual: harus ada tombol Buka kembali, bukan Tutup")
	}

	// tercatat di JSON
	raw, _ := os.ReadFile(filepath.Join(e.dataDir, "events", id+".json"))
	var disk struct {
		ClosedAt *string `json:"closed_at"`
		ClosedBy string  `json:"closed_by"`
	}
	_ = json.Unmarshal(raw, &disk)
	if disk.ClosedAt == nil || !strings.HasPrefix(*disk.ClosedAt, "2026-10-25T15:45:10+07:00") || disk.ClosedBy != "admin" {
		t.Errorf("closed_at/closed_by tidak tercatat: %s", raw)
	}

	// publik: hilang dari beranda, form hilang, pesan jelas, POST ditolak; entri lama tetap terlihat
	if _, body, _ := pub.get("/"); strings.Contains(body, "Arisan") {
		t.Error("event yang ditutup manual tidak boleh tampil di beranda")
	}
	_, body, _ := pub.get(page)
	if strings.Contains(body, `name="amount"`) || !strings.Contains(body, "ditutup oleh admin") || !strings.Contains(body, "Budi") {
		t.Error("halaman event tertutup: form harus hilang, pesan tampil, entri lama tetap terlihat")
	}
	code, body, _ = pub.post(page+"/entries", "/admin/login", entryForm("Sari", "2000", "2026-10-24"))
	if code != http.StatusForbidden || !strings.Contains(body, "ditutup oleh admin") {
		t.Errorf("POST ke event tertutup: %d", code)
	}
	if raw, _ := os.ReadFile(filepath.Join(e.dataDir, "events", id+".json")); strings.Contains(string(raw), "Sari") {
		t.Error("entri ke event tertutup tidak boleh tersimpan")
	}

	// verifikasi tetap bisa untuk event tertutup
	entryID := firstEntryID(t, abody2(t, admin, adminPage))
	if code, _, _ := admin.post(adminPage+"/entries/"+entryID+"/status", adminPage, url.Values{"status": {"valid"}}); code != http.StatusSeeOther {
		t.Errorf("verifikasi event tertutup: %d", code)
	}

	// menutup dua kali tidak error
	if code, _, _ := admin.post(adminPage+"/close", adminPage, nil); code != http.StatusSeeOther {
		t.Errorf("close ulang: %d", code)
	}

	// buka kembali -> aktif lagi
	code, _, _ = admin.post(adminPage+"/reopen", adminPage, nil)
	if code != http.StatusSeeOther {
		t.Fatalf("reopen: %d", code)
	}
	if _, body, _ := pub.get("/"); !strings.Contains(body, "Arisan") {
		t.Error("setelah dibuka kembali event harus muncul di beranda")
	}
	if code, _, _ := pub.post(page+"/entries", page, entryForm("Sari", "2000", "2026-10-24")); code != http.StatusSeeOther {
		t.Errorf("input setelah dibuka kembali: %d", code)
	}
	raw, _ = os.ReadFile(filepath.Join(e.dataDir, "events", id+".json"))
	if !strings.Contains(string(raw), `"closed_at": null`) {
		t.Error("closed_at harus kembali null setelah reopen")
	}
	// reopen saat tidak sedang ditutup: aman
	if code, _, _ := admin.post(adminPage+"/reopen", adminPage, nil); code != http.StatusSeeOther {
		t.Errorf("reopen ulang: %d", code)
	}
}

func abody2(t *testing.T, c *client, path string) string {
	t.Helper()
	_, body, _ := c.get(path)
	return body
}

func TestReopenAfterEndDateWarnsAndStaysClosed(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")
	adminPage := "/admin/e/" + id
	admin.post(adminPage+"/close", adminPage, nil)

	e.setNow("2026-11-03 09:00:00") // tanggal selesai sudah lewat
	// sesi admin dibuat 12:00 hari-25; TTL 12 jam -> login ulang
	admin = e.adminClient()
	admin.post(adminPage+"/reopen", adminPage, nil)
	_, body, _ := admin.get(adminPage)
	if !strings.Contains(body, "tanggal selesai (31 Okt 2026) sudah lewat") {
		t.Error("harus ada peringatan bahwa tanggal selesai sudah lewat")
	}
	if _, pbody, _ := e.newClient().get("/"); strings.Contains(pbody, "Arisan") {
		t.Error("event yang periodenya lewat tidak boleh aktif walau sudah dibuka kembali")
	}
	// perpanjang tanggal selesai -> aktif lagi
	admin.post("/admin/e/"+id, adminPage+"/edit", url.Values{"name": {"Arisan"}, "start_date": {"2026-10-20"}, "end_date": {"2026-11-30"}})
	if _, pbody, _ := e.newClient().get("/"); !strings.Contains(pbody, "Arisan") {
		t.Error("setelah tanggal selesai diperpanjang event harus aktif")
	}
}

func TestCloseDashboardAndAuth(t *testing.T) {
	e := newEnv(t)
	admin := e.adminClient()
	id := e.createEvent(admin, "Arisan", "2026-10-20", "2026-10-31")

	// dari dashboard: kembali ke dashboard
	code, _, h := admin.post("/admin/e/"+id+"/close", "/admin", url.Values{"to": {"dashboard"}})
	if code != http.StatusSeeOther || h.Get("Location") != "/admin" {
		t.Errorf("close dari dashboard: %d %s", code, h.Get("Location"))
	}
	_, body, _ := admin.get("/admin")
	if !strings.Contains(body, "ditutup manual") || !strings.Contains(body, "Buka kembali") {
		t.Error("dashboard harus menandai & menawarkan buka kembali")
	}
	code, _, h = admin.post("/admin/e/"+id+"/reopen", "/admin", url.Values{"to": {"dashboard"}})
	if code != http.StatusSeeOther || h.Get("Location") != "/admin" {
		t.Errorf("reopen dari dashboard: %d %s", code, h.Get("Location"))
	}

	// bukan admin -> dialihkan ke login & tidak berefek
	anon := e.newClient()
	for _, action := range []string{"close", "reopen"} {
		code, _, h := anon.post("/admin/e/"+id+"/"+action, "/admin/login", nil)
		if code != http.StatusSeeOther || !strings.HasPrefix(h.Get("Location"), "/admin/login") {
			t.Errorf("%s tanpa login: %d %s", action, code, h.Get("Location"))
		}
	}
	// tanpa token CSRF -> 403
	if code, _, _ := admin.post("/admin/e/"+id+"/close", "", nil); code != http.StatusForbidden {
		t.Errorf("close tanpa CSRF: %d", code)
	}
	if _, pbody, _ := e.newClient().get("/"); !strings.Contains(pbody, "Arisan") {
		t.Error("event harus tetap aktif: aksi tak sah tidak boleh menutupnya")
	}
	// event tidak ada
	if code, _, _ := admin.post("/admin/e/tidak-ada/close", "/admin", nil); code != http.StatusNotFound {
		t.Errorf("close event tak ada: %d", code)
	}
}
