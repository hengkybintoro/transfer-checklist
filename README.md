# Transfer Checklist

Web sederhana untuk mencatat transfer dana per periode (event) dan memverifikasinya terhadap mutasi rekening. **Tanpa database**: setiap event disimpan sebagai satu file JSON.

- **Publik (tanpa login):** beranda hanya menampilkan **event yang sedang aktif** (bisa beberapa sekaligus); pilih salah satu lalu isi nama, nominal, tanggal transfer, dan lihat semua entri beserta status verifikasinya (Menunggu / Valid / Tidak Valid).
- **Admin (login statis dari config):** buat/ubah event dan rentang tanggal input, **tutup event manual** (dan buka kembali), tandai entri Valid / Tidak Valid, hapus entri spam, unduh CSV.
- **Periode input:** form hanya bisa dikirim dari tanggal mulai sampai tanggal selesai (inklusif) dan selama event belum ditutup manual. Dicek di server.

Spesifikasi lengkap: [docs/PRD.md](docs/PRD.md). Stack: Go (standard library saja), `html/template`, HTML + CSS + sedikit JS.

## Menjalankan di lokal

Butuh Go 1.22+.

```bash
cp config.example.json config.json
# edit config.json: ganti admin.password dan session_secret (minimal 32 karakter acak)
#   contoh membuat secret:  openssl rand -hex 32
go run ./cmd/server -config config.json
```

Buka http://localhost:8080 . Login admin di `/admin/login`. Aplikasi menolak start bila password/secret kosong, secret < 32 karakter, atau masih memakai nilai contoh.

```bash
go test -race ./...        # jalankan test
go build -o transfer-checklist ./cmd/server   # satu binary, template & CSS sudah tertanam
```

## Konfigurasi (`config.json`)

| Field | Default | Keterangan |
|---|---|---|
| `listen_addr` | `:8080` | Alamat listen. Pakai `127.0.0.1:8080` bila di belakang reverse proxy. |
| `base_url` | _(kosong)_ | URL publik. Bila diawali `https://`, cookie diberi flag `Secure`. |
| `data_dir` | `./data` | Folder data; file event ada di `<data_dir>/events/<id>.json`. **Taruh di luar folder repo/workspace runner.** |
| `timezone` | `Asia/Jakarta` | Dipakai untuk batas periode & `submitted_at`. |
| `admin.username` / `admin.password` | – | Wajib. Kredensial statis admin. |
| `session_secret` | – | Wajib, ≥ 32 karakter. Penanda tangan cookie sesi & CSRF. |
| `session_ttl_hours` | `12` | Masa berlaku sesi admin. |
| `trust_proxy` | `false` | Isi `true` **hanya** bila di belakang reverse proxy/tunnel, agar IP klien (untuk rate limit) dibaca dari `CF-Connecting-IP` / `X-Forwarded-For`. |

Setiap nilai bisa ditimpa environment variable: `TC_LISTEN_ADDR`, `TC_BASE_URL`, `TC_DATA_DIR`, `TC_TIMEZONE`, `TC_ADMIN_USERNAME`, `TC_ADMIN_PASSWORD`, `TC_SESSION_SECRET`, `TC_TRUST_PROXY`.

`config.json` dan folder `data/` sudah masuk `.gitignore`. **Jangan pernah commit keduanya.**

## Format data

Satu file per event, contoh `data/events/arisan-oktober-2026.json`. Struktur lengkap ada di [PRD §7.2](docs/PRD.md). File ditulis atomik (tulis `.tmp` → fsync → rename) dan versi sebelumnya disimpan sebagai `.json.bak`. Backup cukup dengan menyalin folder `data_dir`.

## Alur pemakaian

1. Admin login → **Event baru** (nama, tanggal mulai, tanggal selesai, deskripsi/info rekening).
2. Event otomatis tampil di beranda saat periodenya dibuka. Pengirim memilih event dari beranda, atau Anda bisa membagikan link langsung `/e/<id>`.
3. Pengirim mengisi form. Setiap entri berstatus **Menunggu**.
4. Admin membuka `/admin/e/<id>` (urut tanggal transfer, sama seperti membaca mutasi), mencocokkan dengan riwayat rekening, lalu menekan **✓ Valid** atau **✗ Tidak valid** (+ catatan).
5. Halaman publik langsung menampilkan status terbaru.
6. Selesai lebih cepat? Klik **Tutup event** (di dashboard atau halaman event). Event hilang dari beranda dan tidak menerima entri lagi; **Buka kembali** bila perlu. Entri tetap bisa diverifikasi dan event tertutup tetap bisa dibuka lewat link langsungnya.

Catatan: kolom *Catatan* dari pengirim hanya terlihat oleh admin; catatan admin hanya tampil publik untuk entri **Tidak valid**.

## Deployment (self-hosted)

GitHub-hosted runner **tidak** cocok untuk menjalankan web ini (job dibatasi ±6 jam, tanpa URL publik, disk dihapus setelah selesai). Pola yang dipakai:

| Workflow | Runner | Fungsi |
|---|---|---|
| `.github/workflows/ci.yml` | GitHub-hosted | gofmt, vet, test (`-race`), build lintas platform, simpan artifact. Jalan di setiap push & PR. |
| `.github/workflows/deploy.yml` | **self-hosted** | Test → build → pasang binary → restart service → cek `/healthz`. Jalan saat push ke `main` atau manual. |

> **Langkah sekali jalan:** file workflow ada di folder `github-workflows/` (folder `.github/` tidak bisa ditulis oleh Claude). Pindahkan sebelum push:
> ```bash
> mkdir -p .github/workflows && mv github-workflows/*.yml .github/workflows/ && rmdir github-workflows
> ```

**Penting**

- Pakai repo **private**. Self-hosted runner di repo publik berisiko menjalankan kode dari fork.
- `config.json` dan `data_dir` harus di luar workspace runner (checkout membersihkan workspace).
- Rahasia disimpan di `config.json` di mesin tujuan, bukan di repo.

### Langkah setup

1. Buat repo private di GitHub lalu push folder ini.
2. Di mesin yang akan menjalankan web: pasang [self-hosted runner](https://docs.github.com/actions/hosting-your-own-runners) (Settings → Actions → Runners).
3. Siapkan service (pilih salah satu), sekaligus letakkan `config.json` dan folder data:
   - **Linux:** [`deploy/transfer-checklist.service`](deploy/transfer-checklist.service) (systemd). Agar runner boleh restart tanpa password, beri sudoers: `runner ALL=(root) NOPASSWD: /usr/bin/systemctl restart transfer-checklist`.
   - **macOS:** [`deploy/com.transfer-checklist.plist`](deploy/com.transfer-checklist.plist) (launchd). Isi repo **Variables** `INSTALL_DIR=/Users/<user>/transfer-checklist/bin` dan `RESTART_CMD=launchctl kickstart -k gui/$(id -u)/com.transfer-checklist`.
4. (Opsional) Repo **Variables**: `INSTALL_DIR`, `RESTART_CMD`, `HEALTH_URL` bila berbeda dari default (`/opt/transfer-checklist`, `sudo systemctl restart transfer-checklist`, `http://127.0.0.1:8080/healthz`).
5. Push ke `main` → workflow **Deploy** berjalan.

### Akses dari internet

Wajib lewat **HTTPS**. Dua cara mudah:

- **Cloudflare Tunnel** (tanpa buka port): arahkan hostname ke `http://127.0.0.1:8080`.
- **Caddy** sebagai reverse proxy: `transfer.contoh.com { reverse_proxy 127.0.0.1:8080 }` (sertifikat otomatis).

Lalu di `config.json`: `"listen_addr": "127.0.0.1:8080"`, `"base_url": "https://transfer.contoh.com"`, `"trust_proxy": true`.

## Keamanan singkat

Cookie sesi bertanda tangan HMAC (`HttpOnly`, `SameSite=Lax`, `Secure` bila HTTPS) · token CSRF di semua form POST · perbandingan kredensial waktu konstan · rate limit login (5 gagal / 15 menit / IP) dan input entri (10 / menit / IP) · output di-escape `html/template` · header keamanan + CSP · body request dibatasi 16 KB · ID event divalidasi (anti path traversal) · CSV diamankan dari formula injection.

> Halaman publik menampilkan nama dan nominal semua entri. Pastikan para peserta setuju.

## Struktur

```
cmd/server/          entry point
internal/config/     load & validasi config
internal/event/      model + aturan bisnis (periode, validasi, status)
internal/store/      penyimpanan file JSON (atomic write, mutex per event)
internal/auth/       sesi, CSRF, rate limiter
internal/web/        handler HTTP & rendering
web/                 template HTML + CSS/JS (di-embed ke binary)
scripts/deploy.sh    build + pasang + restart + health check
deploy/              contoh service systemd & launchd
github-workflows/    CI dan deploy (pindahkan ke .github/workflows/)
docs/PRD.md          spesifikasi produk
```
