// Package auth berisi sesi admin (cookie bertanda tangan HMAC), token CSRF,
// dan rate limiter sederhana di memori.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// Manager membuat dan memverifikasi token sesi serta token CSRF.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager membuat Manager dengan kunci rahasia dan masa berlaku sesi.
func NewManager(secret string, ttl time.Duration) *Manager {
	return &Manager{secret: []byte(secret), ttl: ttl}
}

// TTL adalah masa berlaku sesi.
func (m *Manager) TTL() time.Duration { return m.ttl }

func (m *Manager) sign(msg string) []byte {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(msg))
	return h.Sum(nil)
}

// NewSession membuat token sesi: base64url("<exp>|<user>") + "." + base64url(hmac).
func (m *Manager) NewSession(user string, now time.Time) string {
	payload := strconv.FormatInt(now.Add(m.ttl).Unix(), 10) + "|" + user
	enc := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return enc + "." + base64.RawURLEncoding.EncodeToString(m.sign("session|"+enc))
}

// VerifySession mengembalikan username bila token sah dan belum kedaluwarsa.
func (m *Manager) VerifySession(token string, now time.Time) (string, bool) {
	enc, sig, ok := strings.Cut(token, ".")
	if !ok {
		return "", false
	}
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, m.sign("session|"+enc)) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return "", false
	}
	expStr, user, ok := strings.Cut(string(raw), "|")
	if !ok {
		return "", false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || now.Unix() >= exp {
		return "", false
	}
	return user, true
}

// CSRFToken menurunkan token form dari nilai cookie CSRF (double-submit
// bertanda tangan): penyerang lintas-situs tidak bisa membaca/menetapkan cookie.
func (m *Manager) CSRFToken(cookieValue string) string {
	return hex.EncodeToString(m.sign("csrf|" + cookieValue))
}

// VerifyCSRF memeriksa token form terhadap nilai cookie.
func (m *Manager) VerifyCSRF(cookieValue, token string) bool {
	if cookieValue == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(m.CSRFToken(cookieValue)), []byte(token)) == 1
}

// SecureEqual membandingkan dua string dalam waktu konstan (panjang tidak bocor).
func SecureEqual(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}
