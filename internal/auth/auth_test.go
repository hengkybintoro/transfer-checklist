package auth

import (
	"strings"
	"testing"
	"time"
)

func TestSession(t *testing.T) {
	m := NewManager("0123456789abcdef0123456789abcdef", time.Hour)
	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)

	tok := m.NewSession("admin|aneh", now) // '|' di username tidak boleh merusak parsing
	if u, ok := m.VerifySession(tok, now.Add(59*time.Minute)); !ok || u != "admin|aneh" {
		t.Errorf("verify = %q, %v", u, ok)
	}
	if _, ok := m.VerifySession(tok, now.Add(time.Hour)); ok {
		t.Error("sesi harus kedaluwarsa tepat di TTL")
	}

	// dirusak
	enc, sig, _ := strings.Cut(tok, ".")
	for _, bad := range []string{"", "x", enc + ".", "." + sig, enc + "." + sig + "A", "A" + enc + "." + sig, tok + tok} {
		if _, ok := m.VerifySession(bad, now); ok {
			t.Errorf("token rusak %q diterima", bad)
		}
	}
	// secret lain
	other := NewManager("ffffffffffffffffffffffffffffffff", time.Hour)
	if _, ok := other.VerifySession(tok, now); ok {
		t.Error("token dari secret lain diterima")
	}
}

func TestCSRF(t *testing.T) {
	m := NewManager("0123456789abcdef0123456789abcdef", time.Hour)
	tok := m.CSRFToken("cookie-a")
	if !m.VerifyCSRF("cookie-a", tok) {
		t.Error("token sah ditolak")
	}
	if m.VerifyCSRF("cookie-b", tok) || m.VerifyCSRF("cookie-a", "salah") || m.VerifyCSRF("", tok) || m.VerifyCSRF("cookie-a", "") {
		t.Error("token tidak sah diterima")
	}
	// token sesi tidak boleh bisa dipakai sebagai token CSRF
	if m.VerifyCSRF("cookie-a", m.NewSession("x", time.Now())) {
		t.Error("domain separation gagal")
	}
}

func TestSecureEqual(t *testing.T) {
	if !SecureEqual("abc", "abc") || SecureEqual("abc", "abd") || SecureEqual("abc", "abcd") || SecureEqual("", "a") {
		t.Error("SecureEqual salah")
	}
}

func TestLimiter(t *testing.T) {
	now := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	l := NewLimiter(3, time.Minute)
	l.SetClock(func() time.Time { return now })

	for i := 0; i < 3; i++ {
		if !l.Allow("ip1") {
			t.Fatalf("Allow #%d seharusnya diizinkan", i+1)
		}
	}
	if l.Allow("ip1") {
		t.Error("kejadian ke-4 harus ditolak")
	}
	if !l.Allow("ip2") {
		t.Error("IP lain tidak boleh terpengaruh")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("ip1") {
		t.Error("setelah jendela lewat harus diizinkan lagi")
	}

	// mode login: Blocked + Fail + Reset
	l2 := NewLimiter(2, time.Minute)
	l2.SetClock(func() time.Time { return now })
	l2.Fail("x")
	if l2.Blocked("x") {
		t.Error("belum mencapai batas")
	}
	l2.Fail("x")
	if !l2.Blocked("x") {
		t.Error("harus terblokir di batas")
	}
	l2.Reset("x")
	if l2.Blocked("x") {
		t.Error("Reset harus membuka blokir")
	}
}
