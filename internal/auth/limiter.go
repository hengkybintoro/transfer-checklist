package auth

import (
	"sync"
	"time"
)

// Limiter membatasi jumlah kejadian per kunci (mis. IP) dalam jendela waktu.
// Disimpan di memori; hilang saat aplikasi restart (cukup untuk MVP).
type Limiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[string][]time.Time
}

// NewLimiter membuat Limiter: maksimal max kejadian per window.
func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{max: max, window: window, now: time.Now, hits: map[string][]time.Time{}}
}

// SetClock mengganti sumber waktu (untuk test).
func (l *Limiter) SetClock(now func() time.Time) { l.now = now }

// prune membuang kejadian di luar jendela. Pemanggil memegang l.mu.
func (l *Limiter) prune(key string) []time.Time {
	cut := l.now().Add(-l.window)
	h := l.hits[key]
	i := 0
	for i < len(h) && !h[i].After(cut) {
		i++
	}
	h = h[i:]
	if len(h) == 0 {
		delete(l.hits, key)
		return nil
	}
	l.hits[key] = h
	return h
}

// Allow mencatat satu kejadian dan mengembalikan false bila batas terlampaui.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 10000 { // sapu sesekali agar map tidak membengkak
		for k := range l.hits {
			l.prune(k)
		}
	}
	if len(l.prune(key)) >= l.max {
		return false
	}
	l.hits[key] = append(l.hits[key], l.now())
	return true
}

// Blocked memeriksa apakah kunci sudah mencapai batas, tanpa mencatat kejadian.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key)) >= l.max
}

// Fail mencatat satu kegagalan (untuk login).
func (l *Limiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(key)
	l.hits[key] = append(l.hits[key], l.now())
}

// Reset menghapus catatan kunci (mis. setelah login berhasil).
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}
