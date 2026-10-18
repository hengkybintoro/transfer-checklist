package config

import (
	"strings"
	"testing"
)

const goodJSON = `{
  "admin": {"username": "admin", "password": "rahasia-banget"},
  "session_secret": "0123456789abcdef0123456789abcdef"
}`

func noEnv(string) string { return "" }

func TestParse_DefaultsAndLocation(t *testing.T) {
	cfg, err := Parse([]byte(goodJSON), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != ":8080" || cfg.DataDir != "./data" || cfg.Timezone != "Asia/Jakarta" || cfg.SessionTTLHours != 12 {
		t.Errorf("default salah: %+v", cfg)
	}
	if cfg.Location == nil || cfg.Location.String() != "Asia/Jakarta" {
		t.Errorf("location = %v", cfg.Location)
	}
	if cfg.Secure() {
		t.Error("tanpa base_url https, Secure harus false")
	}
}

func TestParse_Errors(t *testing.T) {
	cases := map[string]struct{ json, want string }{
		"password kosong": {`{"admin":{"username":"a"},"session_secret":"0123456789abcdef0123456789abcdef"}`, "admin.password"},
		"username kosong": {`{"admin":{"password":"p"},"session_secret":"0123456789abcdef0123456789abcdef"}`, "admin.username"},
		"secret pendek":   {`{"admin":{"username":"a","password":"p"},"session_secret":"pendek"}`, "session_secret"},
		"nilai contoh":    {`{"admin":{"username":"a","password":"ganti-dengan-password-kuat"},"session_secret":"0123456789abcdef0123456789abcdef"}`, "nilai contoh"},
		"timezone salah":  {`{"admin":{"username":"a","password":"p"},"session_secret":"0123456789abcdef0123456789abcdef","timezone":"Mars/Olympus"}`, "timezone"},
		"base_url salah":  {`{"admin":{"username":"a","password":"p"},"session_secret":"0123456789abcdef0123456789abcdef","base_url":"ftp://x"}`, "base_url"},
		"field typo":      {`{"admin":{"username":"a","password":"p"},"sesion_secret":"x"}`, "unknown field"},
		"bukan json":      {`{`, "config tidak valid"},
	}
	for name, c := range cases {
		_, err := Parse([]byte(c.json), noEnv)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want mengandung %q", name, err, c.want)
		}
	}
}

func TestParse_EnvOverrideAndSecure(t *testing.T) {
	env := map[string]string{
		"TC_ADMIN_PASSWORD": "dari-env",
		"TC_SESSION_SECRET": "ffffffffffffffffffffffffffffffff",
		"TC_DATA_DIR":       "/var/lib/tc",
		"TC_LISTEN_ADDR":    "127.0.0.1:9000",
		"TC_BASE_URL":       "https://transfer.contoh.com",
		"TC_TRUST_PROXY":    "true",
	}
	cfg, err := Parse([]byte(`{"admin":{"username":"admin","password":"x"}}`), func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Admin.Password != "dari-env" || cfg.DataDir != "/var/lib/tc" || cfg.ListenAddr != "127.0.0.1:9000" || !cfg.TrustProxy || !cfg.Secure() {
		t.Errorf("override env salah: %+v", cfg)
	}
}
