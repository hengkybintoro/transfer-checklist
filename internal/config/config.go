// Package config memuat dan memvalidasi konfigurasi aplikasi dari config.json
// dengan override lewat environment variable.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // zona waktu tertanam, tidak bergantung pada OS
)

// Admin adalah kredensial statis untuk satu akun admin.
type Admin struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Config adalah seluruh konfigurasi aplikasi.
type Config struct {
	ListenAddr      string `json:"listen_addr"`
	BaseURL         string `json:"base_url"`
	DataDir         string `json:"data_dir"`
	Timezone        string `json:"timezone"`
	Admin           Admin  `json:"admin"`
	SessionSecret   string `json:"session_secret"`
	SessionTTLHours int    `json:"session_ttl_hours"`
	// TrustProxy: percaya header CF-Connecting-IP / X-Forwarded-For untuk
	// menentukan IP klien (aktifkan hanya bila di belakang reverse proxy).
	TrustProxy bool `json:"trust_proxy"`

	Location *time.Location `json:"-"`
}

// Secure true bila aplikasi diakses lewat HTTPS (cookie diberi flag Secure).
func (c *Config) Secure() bool {
	return strings.HasPrefix(strings.ToLower(c.BaseURL), "https://")
}

// SessionTTL mengembalikan masa berlaku sesi admin.
func (c *Config) SessionTTL() time.Duration {
	return time.Duration(c.SessionTTLHours) * time.Hour
}

// Load membaca file config dari path lalu menerapkan override env.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tidak bisa membaca config %q: %w (salin config.example.json menjadi config.json)", path, err)
	}
	return Parse(data, os.Getenv)
}

// Parse mem-parsing isi config, menerapkan default, override env, dan validasi.
func Parse(data []byte, getenv func(string) string) (*Config, error) {
	cfg := &Config{}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // menangkap salah ketik nama field
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("config tidak valid: %w", err)
	}
	cfg.applyEnv(getenv)
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyEnv(getenv func(string) string) {
	set := func(dst *string, key string) {
		if v := getenv(key); v != "" {
			*dst = v
		}
	}
	set(&c.ListenAddr, "TC_LISTEN_ADDR")
	set(&c.BaseURL, "TC_BASE_URL")
	set(&c.DataDir, "TC_DATA_DIR")
	set(&c.Timezone, "TC_TIMEZONE")
	set(&c.Admin.Username, "TC_ADMIN_USERNAME")
	set(&c.Admin.Password, "TC_ADMIN_PASSWORD")
	set(&c.SessionSecret, "TC_SESSION_SECRET")
	if v := getenv("TC_TRUST_PROXY"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			c.TrustProxy = b
		}
	}
}

func (c *Config) applyDefaults() {
	if c.ListenAddr == "" {
		c.ListenAddr = ":8080"
	}
	if c.DataDir == "" {
		c.DataDir = "./data"
	}
	if c.Timezone == "" {
		c.Timezone = "Asia/Jakarta"
	}
	if c.SessionTTLHours == 0 {
		c.SessionTTLHours = 12
	}
}

func (c *Config) validate() error {
	if strings.TrimSpace(c.Admin.Username) == "" {
		return fmt.Errorf("config: admin.username wajib diisi")
	}
	if c.Admin.Password == "" {
		return fmt.Errorf("config: admin.password wajib diisi")
	}
	if len(c.SessionSecret) < 32 {
		return fmt.Errorf("config: session_secret wajib diisi dan minimal 32 karakter")
	}
	if strings.HasPrefix(c.Admin.Password, "ganti-dengan") || strings.HasPrefix(c.SessionSecret, "ganti-dengan") {
		return fmt.Errorf("config: admin.password / session_secret masih memakai nilai contoh, ganti dulu")
	}
	if c.SessionTTLHours < 1 {
		return fmt.Errorf("config: session_ttl_hours harus >= 1")
	}
	if c.BaseURL != "" {
		u, err := url.Parse(c.BaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("config: base_url harus berupa URL http(s) yang valid")
		}
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return fmt.Errorf("config: timezone %q tidak dikenal: %w", c.Timezone, err)
	}
	c.Location = loc
	return nil
}
