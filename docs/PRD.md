# PRD — Transfer Checklist

| | |
|---|---|
| **Versi** | 1.0 (MVP) |
| **Tanggal** | 18 Oktober 2026 |
| **Pemilik** | Strapa |
| **Status** | Draft untuk implementasi |

---

## 1. Ringkasan

Transfer Checklist adalah web sederhana (bisa dibuka di desktop, laptop, dan HP) tempat orang mencatat sendiri transfer yang mereka kirim ke rekening pemilik: **nama, nominal, dan tanggal transfer**. Sistem otomatis menyimpan waktu data diinput. Admin login, mencocokkan setiap entri dengan mutasi rekening, lalu menandainya **Valid** atau **Tidak Valid**. Semua orang dapat melihat daftar entri beserta status verifikasinya tanpa login.

Pencatatan dibagi per **periode (event)** dengan rentang tanggal input tertentu. Setiap event disimpan dalam **satu file JSON** sendiri. Tidak ada database.

## 2. Latar Belakang & Masalah

Saat ini alurnya:

1. Orang mentransfer dana ke rekening pemilik.
2. Orang tersebut mengabari lewat chat.
3. Pemilik membaca chat satu per satu, lalu mencocokkannya secara manual dengan riwayat transaksi rekening.

Masalahnya:

- Informasi tersebar di banyak chat, formatnya tidak seragam (ada yang lupa nominal, ada yang lupa tanggal).
- Sulit melihat rekap: siapa yang sudah transfer, berapa totalnya, mana yang sudah dicek.
- Orang yang transfer tidak tahu apakah transfernya sudah diterima/dicek, sehingga sering bertanya ulang.

## 3. Tujuan & Non-Tujuan

### Tujuan

- **G1** — Satu tempat terpusat untuk pencatatan transfer dengan format seragam.
- **G2** — Admin bisa memverifikasi entri dengan cepat, diurutkan/difilter berdasarkan tanggal transfer agar mudah dicocokkan dengan mutasi rekening.
- **G3** — Transparansi: siapa pun bisa melihat status verifikasi entrinya tanpa harus bertanya.
- **G4** — Periode input dibatasi; di luar rentang tanggal event, form input tertutup. Admin juga bisa menutup event kapan saja secara manual.
- **G6** — Beberapa event bisa aktif bersamaan; pengunjung hanya melihat event yang sedang aktif dan memilih salah satu untuk diisi.
- **G5** — Sangat sederhana untuk dijalankan: satu binary, tanpa database, data berupa file JSON yang mudah dibaca dan di-backup.

### Non-Tujuan (di luar scope MVP)

- Integrasi otomatis ke API bank / mutasi rekening.
- Upload bukti transfer (gambar).
- Akun/login untuk pengguna umum.
- Multi-admin dengan peran berbeda.
- Notifikasi (email, WhatsApp, dsb.).
- Pembayaran atau pemindahan dana apa pun.

## 4. Peran Pengguna

| Peran | Deskripsi | Login |
|---|---|---|
| **Pengirim (publik)** | Orang yang mentransfer dana dan mencatatnya. | Tidak |
| **Pengunjung (publik)** | Siapa pun yang ingin melihat daftar entri dan statusnya. | Tidak |
| **Admin** | Pemilik rekening. Mengelola event dan memverifikasi entri. | Ya (static, dari file config) |

## 5. User Stories

### Publik

- **US-01** — Sebagai pengirim, saya ingin mengisi nama, nominal, dan tanggal transfer di form sederhana agar tidak perlu mengabari lewat chat.
- **US-02** — Sebagai pengirim, saya ingin melihat pesan jelas bila periode input belum dibuka atau sudah ditutup.
- **US-03** — Sebagai pengunjung, saya ingin melihat semua entri pada sebuah event beserta status verifikasinya (Menunggu / Valid / Tidak Valid).
- **US-05** — Sebagai pengirim, saya ingin beranda hanya menampilkan event yang sedang aktif, dan bila ada beberapa, saya bisa memilih event yang sesuai untuk transfer saya.
- **US-04** — Sebagai pengirim, saya ingin mencari nama saya di daftar agar cepat tahu status transfer saya.

### Admin

- **US-10** — Sebagai admin, saya ingin login dengan username dan password yang ada di file konfigurasi lokal.
- **US-11** — Sebagai admin, saya ingin membuat event baru dengan nama, tanggal mulai, dan tanggal selesai.
- **US-12** — Sebagai admin, saya ingin mengubah nama/tanggal event.
- **US-17** — Sebagai admin, saya ingin **menutup event secara manual** tanpa menunggu tanggal selesai, dan bisa **membukanya kembali** bila salah tutup.
- **US-13** — Sebagai admin, saya ingin melihat entri yang diurutkan dan difilter berdasarkan tanggal transfer serta status, agar mudah dicocokkan dengan mutasi rekening.
- **US-14** — Sebagai admin, saya ingin menandai entri sebagai **Valid** atau **Tidak Valid** (dengan catatan opsional), dan bisa mengembalikannya ke **Menunggu** bila salah klik.
- **US-15** — Sebagai admin, saya ingin melihat rekap: jumlah entri dan total nominal per status.
- **US-16** — Sebagai admin, saya ingin menghapus entri spam/duplikat.

## 6. Kebutuhan Fungsional

### 6.1 Halaman Publik

| ID | Kebutuhan |
|---|---|
| **FR-01** | Halaman beranda (`/`) hanya menampilkan event yang **sedang aktif** (dalam rentang tanggal **dan** belum ditutup manual): nama, deskripsi, tanggal berakhir, jumlah entri. Event yang belum dibuka, sudah lewat, atau ditutup manual **tidak** tampil. Boleh ada beberapa event aktif sekaligus; urutannya paling cepat berakhir di atas. Bila tidak ada, tampil "Belum ada event yang aktif saat ini". Halaman event (`/e/{id}`) tetap bisa dibuka lewat link langsung (mis. untuk melihat status verifikasi event yang sudah ditutup), tetapi tanpa form input. |
| **FR-02** | Halaman event (`/e/{id}`) berisi form input (bila periode dibuka) dan tabel semua entri event tersebut. |
| **FR-03** | Field form input: **Nama** (wajib), **Nominal** (wajib, Rupiah), **Tanggal transfer** (wajib, date picker), **Catatan** (opsional, mis. "transfer dari rekening istri"). |
| **FR-04** | Sistem otomatis mencatat `submitted_at` (waktu server saat data diterima). Nilai ini tidak bisa diisi/diubah oleh pengguna. |
| **FR-05** | Form hanya bisa dikirim bila waktu server berada dalam periode event. Validasi dilakukan **di server**, bukan hanya di browser. Di luar periode, form disembunyikan dan diganti pesan "Periode input belum dibuka (mulai DD MMM YYYY)" atau "Periode input sudah ditutup (DD MMM YYYY)". Untuk event yang ditutup manual: "Event ini sudah ditutup oleh admin…". |
| **FR-06** | Setelah berhasil, tampilkan pesan konfirmasi berisi ringkasan data yang dikirim dan kembali ke halaman event (pola Post/Redirect/Get agar refresh tidak mengirim ulang). |
| **FR-07** | Tabel entri menampilkan: Nama, Nominal, Tanggal transfer, Waktu input, dan Status (badge berwarna: abu-abu = Menunggu, hijau ✓ = Valid, merah ✗ = Tidak Valid). Catatan admin untuk entri Tidak Valid ditampilkan (mis. "tidak ditemukan di mutasi tgl 21/10"). |
| **FR-08** | Tabel bisa dicari berdasarkan nama (filter sisi klien, tanpa reload) dan diurutkan berdasarkan waktu input terbaru. |
| **FR-09** | Ringkasan di atas tabel: jumlah entri dan total nominal yang **Valid**. |
| **FR-10** | Pengguna publik **tidak bisa** mengubah atau menghapus entri yang sudah dikirim. Bila salah input, kirim entri baru dan beri tahu admin. |

### 6.2 Admin

| ID | Kebutuhan |
|---|---|
| **FR-20** | Halaman login (`/admin/login`) dengan username & password. Kredensial dibaca dari file konfigurasi lokal saat aplikasi start. |
| **FR-21** | Setelah login, sesi disimpan dalam cookie bertanda tangan (HMAC) dengan masa berlaku yang bisa dikonfigurasi (default 12 jam). Ada tombol Logout. |
| **FR-22** | Dashboard admin (`/admin`) menampilkan semua event beserta rekap per event: jumlah Menunggu / Valid / Tidak Valid dan total nominal masing-masing. |
| **FR-23** | Admin dapat membuat event: **Nama**, **Tanggal mulai**, **Tanggal selesai**, **Deskripsi** (opsional, mis. info rekening tujuan). Sistem membuat ID (slug dari nama, mis. `arisan-oktober-2026`) dan file JSON baru. |
| **FR-24** | Admin dapat mengubah nama, deskripsi, tanggal mulai/selesai event. Mengubah tanggal tidak menghapus entri yang sudah ada. |
| **FR-30** | Admin dapat **menutup event secara manual** (tombol "Tutup event" di halaman event dan di dashboard, dengan konfirmasi). Sejak itu form input langsung tidak tersedia, POST entri ditolak (HTTP 403), dan event hilang dari beranda publik. Sistem mencatat `closed_at` dan `closed_by`. Menutup event yang sudah ditutup tidak menimbulkan error. |
| **FR-31** | Admin dapat **membuka kembali** event yang ditutup manual (tombol "Buka kembali"). Status event kembali mengikuti rentang tanggal; bila tanggal selesai sudah lewat, admin diberi peringatan agar memperpanjang tanggal selesai (event baru aktif lagi setelah itu). Verifikasi entri tetap bisa dilakukan pada event yang ditutup. |
| **FR-25** | Halaman verifikasi event (`/admin/e/{id}`) menampilkan tabel entri dengan filter status dan urutan default **tanggal transfer naik**, lalu waktu input — sama dengan urutan saat membaca mutasi rekening. |
| **FR-26** | Setiap baris memiliki aksi: **✓ Valid**, **✗ Tidak Valid**, **↺ Reset ke Menunggu**, dan **Hapus**. Aksi Tidak Valid membuka input catatan opsional. Aksi Hapus meminta konfirmasi. |
| **FR-27** | Saat status diubah, sistem mencatat `verified_at` dan `verified_by` (username admin). |
| **FR-28** | Sistem menandai entri yang **kemungkinan duplikat** (nama sama tanpa membedakan huruf besar/kecil, nominal sama, tanggal transfer sama) dengan label "Kemungkinan duplikat" di halaman admin. Hanya penanda; tidak memblokir input. |
| **FR-29** | Admin dapat mengunduh data event sebagai **CSV** (untuk dicocokkan di spreadsheet bila perlu). File JSON aslinya juga tetap bisa diambil langsung dari folder data. |

### 6.3 Aturan Bisnis

| ID | Aturan |
|---|---|
| **BR-01** | Semua tanggal & waktu memakai zona waktu dari config (default `Asia/Jakarta`). |
| **BR-02** | Periode event **inklusif**: dibuka mulai `start_date 00:00:00` dan ditutup setelah `end_date 23:59:59` (zona waktu config). |
| **BR-03** | `end_date` harus ≥ `start_date`. |
| **BR-04** | Nama: 2–100 karakter setelah di-trim; spasi berlebih dirapikan. |
| **BR-05** | Nominal: bilangan bulat Rupiah, 1 s.d. 1.000.000.000. Input menerima format `150000`, `150.000`, atau `150,000`; disimpan sebagai angka. Ditampilkan sebagai `Rp150.000`. |
| **BR-06** | Tanggal transfer tidak boleh di masa depan (relatif ke tanggal server). Tanggal transfer **boleh** sebelum `start_date` (orang bisa transfer lebih awal), tetapi tidak boleh lebih dari 31 hari sebelum `start_date`. |
| **BR-07** | Catatan pengirim maksimal 200 karakter; catatan admin maksimal 200 karakter. |
| **BR-08** | Status entri: `pending` (default) → `valid` atau `invalid`. Admin dapat mengubah ke status mana pun, termasuk kembali ke `pending`. |
| **BR-09** | Event lebih dari satu boleh aktif bersamaan; tiap event punya halaman dan file sendiri. Pengirim memilih event dari beranda. |
| **BR-10** | Event yang sudah ditutup (otomatis maupun manual) tetap bisa dilihat publik lewat link langsung dan diverifikasi admin, tetapi tidak tampil di beranda dan tidak menerima entri baru. |
| **BR-11** | **Event aktif** = tidak ditutup manual **dan** waktu server berada dalam rentang `start_date`–`end_date`. Penutupan manual selalu mengalahkan rentang tanggal (termasuk untuk event yang belum dimulai). |
| **BR-12** | Membuka kembali event tidak mengubah tanggal: event yang tanggal selesainya sudah lewat tetap tidak aktif sampai admin memperpanjang tanggal selesai. |

## 7. Penyimpanan Data (Tanpa Database)

### 7.1 Struktur folder data

```
<data_dir>/
└── events/
    ├── arisan-oktober-2026.json
    ├── iuran-reuni-2026.json
    └── ...
```

- **Satu file JSON per event.** Daftar event dibangun dengan membaca semua file `*.json` di folder `events/` saat start dan disimpan di memori.
- `data_dir` diatur lewat config dan **harus berada di luar folder repo/checkout**, supaya proses deploy atau `git clean` tidak menghapus data.

### 7.2 Format file event

```json
{
  "id": "arisan-oktober-2026",
  "name": "Arisan Oktober 2026",
  "description": "Transfer ke BCA 123xxxx a.n. Strapa",
  "start_date": "2026-10-20",
  "end_date": "2026-10-31",
  "created_at": "2026-10-18T10:00:00+07:00",
  "updated_at": "2026-10-21T09:14:03+07:00",
  "closed_at": null,
  "closed_by": "",
  "entries": [
    {
      "id": "8f3a2c1d9b7e",
      "name": "Budi Santoso",
      "amount": 150000,
      "transfer_date": "2026-10-21",
      "note": "",
      "submitted_at": "2026-10-21T09:14:03+07:00",
      "status": "pending",
      "admin_note": "",
      "verified_at": null,
      "verified_by": ""
    }
  ]
}
```

| Field | Tipe | Keterangan |
|---|---|---|
| `id` (event) | string | Slug unik, juga nama file. Huruf kecil, angka, tanda `-`. |
| `start_date`, `end_date` | string `YYYY-MM-DD` | Rentang periode input. |
| `closed_at` | string RFC 3339 \| null | Diisi saat admin menutup event manual; `null` bila tidak ditutup manual. File lama tanpa field ini dianggap `null`. |
| `closed_by` | string | Username admin yang menutup event. |
| `id` (entri) | string | 12 karakter hex acak (crypto/rand). |
| `amount` | integer | Rupiah, tanpa desimal. |
| `transfer_date` | string `YYYY-MM-DD` | Diisi pengirim. |
| `submitted_at` | string RFC 3339 | Diisi server. |
| `status` | `pending` \| `valid` \| `invalid` | Status verifikasi. |
| `verified_at` | string RFC 3339 \| null | Waktu terakhir status diubah admin. |

### 7.3 Keamanan penulisan file

- **Satu proses** aplikasi, dengan `sync.Mutex` per event agar dua request tidak menulis file yang sama bersamaan.
- **Atomic write**: tulis ke `<id>.json.tmp`, `fsync`, lalu `rename` ke `<id>.json`. File tidak pernah setengah tertulis meski server mati mendadak.
- **Backup ringan**: sebelum menimpa, salin versi sebelumnya ke `<id>.json.bak` (satu generasi). Backup lebih lengkap cukup dengan menyalin folder `data_dir` (cron/rsync), di luar scope aplikasi.
- JSON ditulis dengan indentasi (pretty-print) supaya mudah dibaca dan di-diff.

### 7.4 Batas skala

Target: sampai ±50 event dan ±2.000 entri per event. Pada skala ini, membaca/menulis ulang seluruh file per perubahan masih jauh di bawah 50 ms. Bila jauh melampaui ini, pertimbangkan SQLite (di luar scope MVP).

## 8. Konfigurasi

File `config.json` (lokasi default `./config.json`, bisa diganti dengan flag `-config`). **File ini tidak di-commit ke Git**; repo hanya berisi `config.example.json`.

```json
{
  "listen_addr": ":8080",
  "base_url": "https://transfer.contoh.com",
  "data_dir": "/var/lib/transfer-checklist",
  "timezone": "Asia/Jakarta",
  "admin": {
    "username": "admin",
    "password": "ganti-dengan-password-kuat"
  },
  "session_secret": "string-acak-minimal-32-karakter",
  "session_ttl_hours": 12,
  "trust_proxy": false
}
```

- `trust_proxy` (tambahan implementasi): isi `true` hanya bila aplikasi berada di belakang reverse proxy/tunnel, agar IP klien untuk rate limit dibaca dari `CF-Connecting-IP` / `X-Forwarded-For`. Tanpa ini semua pengunjung tampak berasal dari IP proxy.
- Aplikasi **menolak start** bila `admin.password` atau `session_secret` kosong, atau `session_secret` < 32 karakter.
- Field `admin.password` berupa teks biasa (sesuai kebutuhan "static login"). Dukungan hash bcrypt (opsional di PRD) **tidak** diimplementasikan di MVP agar tanpa dependency eksternal.
- Nilai config bisa ditimpa environment variable (`TC_ADMIN_PASSWORD`, `TC_SESSION_SECRET`, `TC_DATA_DIR`, `TC_LISTEN_ADDR`) agar rahasia bisa disimpan di GitHub Secrets atau environment mesin.

## 9. Arsitektur & Tech Stack

### 9.1 Pilihan teknologi

| Komponen | Pilihan | Alasan |
|---|---|---|
| Bahasa | **Go 1.22+** | Satu binary, cepat, mudah di-cross-compile; routing bawaan sudah mendukung method & path parameter. |
| Web server | `net/http` (standard library) | Tanpa framework. |
| Template | `html/template` | Auto-escaping, aman dari XSS. |
| Aset | `embed` | Template & CSS ikut di dalam binary; deploy cukup satu file + config. |
| Frontend | HTML + CSS sederhana + sedikit vanilla JS | Responsif untuk mobile, tanpa build step. JS hanya untuk pencarian nama dan dialog konfirmasi. |
| Penyimpanan | File JSON per event | Sesuai kebutuhan: tanpa database. |
| Dependency eksternal | **Tidak ada** (opsional `golang.org/x/crypto/bcrypt` bila memakai hash) | Minim maintenance. |

### 9.2 Struktur repo yang diusulkan

```
transfer-checklist/
├── cmd/server/main.go          # entry point: baca config, start HTTP server
├── internal/
│   ├── config/                 # load & validasi config.json + env override
│   ├── store/                  # baca/tulis file JSON event, mutex, atomic write
│   ├── event/                  # model Event & Entry, aturan bisnis & validasi
│   ├── auth/                   # login, cookie sesi (HMAC), CSRF token
│   └── web/                    # handler HTTP, render template
├── web/
│   ├── templates/              # *.html (layout, home, event, admin, login)
│   └── static/                 # style.css, app.js
├── docs/PRD.md
├── config.example.json
├── .github/workflows/ci.yml    # test & build
├── .github/workflows/deploy.yml# deploy ke self-hosted runner
├── .gitignore                  # config.json, data/, binary
├── go.mod
└── README.md
```

### 9.3 Daftar route

| Method | Path | Akses | Fungsi |
|---|---|---|---|
| GET | `/` | Publik | Daftar event |
| GET | `/e/{id}` | Publik | Form input + tabel entri |
| POST | `/e/{id}/entries` | Publik | Kirim entri baru |
| GET | `/admin/login` | Publik | Form login |
| POST | `/admin/login` | Publik | Proses login |
| POST | `/admin/logout` | Admin | Logout |
| GET | `/admin` | Admin | Dashboard semua event |
| GET | `/admin/events/new` | Admin | Form event baru |
| POST | `/admin/events` | Admin | Buat event |
| GET | `/admin/e/{id}` | Admin | Halaman verifikasi entri |
| GET | `/admin/e/{id}/edit` | Admin | Form ubah event |
| POST | `/admin/e/{id}` | Admin | Simpan perubahan event |
| POST | `/admin/e/{id}/close` | Admin | Tutup event secara manual |
| POST | `/admin/e/{id}/reopen` | Admin | Buka kembali event yang ditutup manual |
| POST | `/admin/e/{id}/entries/{entryID}/status` | Admin | Ubah status (`valid`/`invalid`/`pending`) + catatan |
| POST | `/admin/e/{id}/entries/{entryID}/delete` | Admin | Hapus entri |
| GET | `/admin/e/{id}/export.csv` | Admin | Unduh CSV |
| GET | `/healthz` | Publik | Health check (`200 OK`) |

Semua aksi memakai form HTML biasa (POST + redirect), sehingga tetap berfungsi tanpa JavaScript.

## 10. Keamanan

| ID | Kebutuhan |
|---|---|
| **SEC-01** | Perbandingan username/password memakai `crypto/subtle.ConstantTimeCompare`. |
| **SEC-02** | Cookie sesi: `HttpOnly`, `SameSite=Lax`, `Secure` (bila `base_url` HTTPS), ditandatangani HMAC-SHA256 dengan `session_secret`, berisi username + waktu kedaluwarsa. |
| **SEC-03** | Semua form POST memakai token CSRF. |
| **SEC-04** | Rate limit sederhana di memori: login maks. 5 percobaan gagal per IP per 15 menit; input entri maks. 10 per IP per menit. |
| **SEC-05** | Semua output di-escape oleh `html/template`. Input di-trim dan dibatasi panjangnya. Ukuran body request dibatasi (mis. 16 KB). |
| **SEC-06** | `config.json` dan folder data masuk `.gitignore`. Permission file config `0600`. |
| **SEC-07** | Bila diakses dari internet, wajib lewat **HTTPS** (reverse proxy seperti Caddy, atau Cloudflare Tunnel). |
| **SEC-08** | Header keamanan dasar: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: same-origin`. |

> **Catatan privasi.** Halaman publik menampilkan nama dan nominal semua orang. Pastikan peserta event setuju dengan hal ini. Bila perlu, versi berikutnya bisa menambahkan opsi per event untuk menyamarkan nama (`Budi S***`) atau menyembunyikan nominal di tampilan publik.

## 11. Kebutuhan Non-Fungsional

| ID | Kebutuhan |
|---|---|
| **NFR-01** | **Responsif**: nyaman dipakai di layar 360 px (HP) hingga desktop. Tabel berubah menjadi kartu per entri di layar kecil. |
| **NFR-02** | **Ringan**: halaman < 100 KB, tanpa framework JS/CSS, tanpa CDN eksternal. |
| **NFR-03** | **Mudah dijalankan**: `go run ./cmd/server` atau satu binary + `config.json`. |
| **NFR-04** | **Tahan crash**: atomic write; data tidak korup bila proses mati di tengah penulisan. |
| **NFR-05** | **Teruji**: unit test untuk aturan bisnis (periode, validasi nominal & tanggal, transisi status) dan store (atomic write, concurrency). |
| **NFR-06** | **Bahasa UI**: Bahasa Indonesia. Format angka `Rp150.000`, format tanggal `21 Okt 2026`. |
| **NFR-07** | **Log**: akses & aksi admin dicatat ke stdout (siapa, kapan, entri apa, status lama → baru). |

## 12. Deployment & GitHub

### 12.1 Kenapa bukan GitHub-hosted runner

GitHub-hosted runner hanya cocok untuk **build & test**. Untuk menjalankan web, runner ini:

- dibatasi maksimal ±6 jam per job,
- tidak punya alamat publik yang bisa diakses orang lain,
- menghapus seluruh disk (termasuk file JSON) setelah job selesai.

### 12.2 Rencana yang dipilih: self-hosted

- Aplikasi berjalan di **mesin sendiri** (VPS, mini PC, atau laptop yang selalu menyala) sebagai service (`systemd` di Linux / `launchd` di macOS) agar otomatis start ulang.
- Data JSON tersimpan permanen di `data_dir` mesin tersebut.
- Akses dari internet melalui reverse proxy HTTPS (Caddy) atau Cloudflare Tunnel.

### 12.3 Workflow GitHub Actions

**`ci.yml`** — jalan di GitHub-hosted runner, setiap push & pull request:

1. `go vet ./...`
2. `go test ./...`
3. `go build` untuk `linux/amd64`, `linux/arm64`, `darwin/arm64`
4. Simpan binary sebagai artifact

**`deploy.yml`** — jalan di **self-hosted runner** milik sendiri, saat push ke `main` (atau manual via `workflow_dispatch`):

1. Build binary.
2. Salin binary ke lokasi instalasi (mis. `/opt/transfer-checklist/`).
3. Restart service.
4. Cek `/healthz`.

Catatan penting:

- `data_dir` dan `config.json` berada **di luar** workspace runner, karena `actions/checkout` membersihkan workspace setiap job.
- Disarankan memakai **repo private**. Self-hosted runner pada repo publik berisiko: pull request dari fork bisa menjalankan kode di mesin Anda.
- Rahasia (`TC_ADMIN_PASSWORD`, `TC_SESSION_SECRET`) disimpan di file config mesin atau di environment service, bukan di repo.

## 13. Gambaran Halaman

**Beranda (`/`)**

```
┌─────────────────────────────────────┐
│ Transfer Checklist                  │
├─────────────────────────────────────┤
│ Event Aktif                         │
│ ┌─────────────────────────────────┐ │
│ │ Arisan Oktober 2026    [Dibuka] │ │
│ │ Transfer ke BCA 123xxxx ...     │ │
│ │ Input dibuka sampai 31 Okt 2026 │ │
│ └─────────────────────────────────┘ │
│ ┌─────────────────────────────────┐ │
│ │ Iuran Reuni 2026       [Dibuka] │ │
│ │ Input dibuka sampai 5 Nov 2026  │ │
│ └─────────────────────────────────┘ │
└─────────────────────────────────────┘
 (hanya event aktif; beberapa sekaligus)
```

**Halaman event (`/e/{id}`)**

```
┌─────────────────────────────────────┐
│ Arisan Oktober 2026                 │
│ Dibuka 20 Okt – 31 Okt 2026         │
│ Transfer ke BCA 123xxxx a.n. Strapa │
├─────────────────────────────────────┤
│ Nama            [______________]    │
│ Nominal (Rp)    [______________]    │
│ Tanggal transfer[ 21/10/2026  ▾]    │
│ Catatan         [______________]    │
│                 [   Kirim   ]       │
├─────────────────────────────────────┤
│ 12 entri · Valid: Rp1.800.000       │
│ Cari nama: [________]               │
│ Nama   Nominal    Tgl TF   Status   │
│ Budi   150.000    21 Okt   ✓ Valid  │
│ Sari   150.000    21 Okt   ⏳ Menunggu│
│ Andi   100.000    22 Okt   ✗ Tidak  │
│        valid — tidak ada di mutasi  │
└─────────────────────────────────────┘
```

**Verifikasi admin (`/admin/e/{id}`)**

```
┌─────────────────────────────────────────────┐
│ Arisan Oktober 2026      [Ubah] [CSV] [Logout]│
│ Menunggu 5 (Rp750rb) · Valid 6 · Tidak 1     │
│ Filter: [Semua ▾]  Urut: Tanggal transfer ▲  │
├─────────────────────────────────────────────┤
│ 21 Okt │ Budi │ 150.000 │ ✓Valid             │
│ 21 Okt │ Sari │ 150.000 │ ⏳ [✓] [✗] [Hapus]   │
│ 22 Okt │ Andi │ 100.000 │ ✗ [↺]  ⚠ duplikat?  │
└─────────────────────────────────────────────┘
```

## 14. Kriteria Penerimaan (Acceptance Criteria)

- **AC-01** — Saat event dibuka, pengguna di HP dapat mengirim entri valid dan langsung melihatnya di tabel dengan status **Menunggu** dan `submitted_at` terisi otomatis.
- **AC-02** — Sehari sebelum `start_date` dan sehari setelah `end_date`, form tidak tampil, dan POST langsung ke `/e/{id}/entries` (mis. via curl) ditolak dengan HTTP 403 dan pesan yang sesuai.
- **AC-03** — Input dengan nominal `0`, nominal bukan angka, nama kosong, atau tanggal transfer di masa depan ditolak dengan pesan error per field, dan nilai yang sudah diisi tidak hilang.
- **AC-04** — Login dengan kredensial salah gagal; kredensial dari `config.json` berhasil; semua halaman `/admin/*` mengarahkan ke login bila belum login.
- **AC-05** — Admin membuat event baru → file `<data_dir>/events/<id>.json` muncul dengan isi sesuai format §7.2.
- **AC-06** — Admin menandai entri **Valid** → badge hijau ✓ tampil di halaman publik setelah refresh; `verified_at` dan `verified_by` tercatat di file JSON.
- **AC-07** — Admin menandai entri **Tidak Valid** dengan catatan → badge merah ✗ dan catatan tampil di halaman publik.
- **AC-08** — 50 entri dikirim bersamaan (load test sederhana) → tidak ada entri yang hilang dan file JSON tetap valid.
- **AC-09** — Proses dihentikan paksa (`kill -9`) di tengah penulisan → setelah start ulang, file JSON tetap bisa dibaca (versi lama atau baru, tidak korup).
- **AC-10** — Aplikasi tidak mau start bila `config.json` tidak ada, password kosong, atau `session_secret` terlalu pendek, dan menampilkan pesan error yang jelas.
- **AC-11** — Workflow CI hijau (vet, test, build) pada setiap push.
- **AC-12** — Dengan dua event aktif, beranda publik menampilkan keduanya (paling cepat berakhir di atas) dan tidak menampilkan event yang belum dibuka, sudah lewat, atau ditutup manual.
- **AC-13** — Admin menekan "Tutup event" pada event yang masih dalam rentang tanggal → event hilang dari beranda, form di `/e/{id}` hilang, POST entri ditolak 403, `closed_at` dan `closed_by` tercatat di JSON; entri lama tetap terlihat.
- **AC-14** — "Buka kembali" mengaktifkan event lagi bila masih dalam rentang tanggal; bila tanggal selesai sudah lewat, admin mendapat peringatan dan event tetap tidak aktif sampai tanggal selesai diperpanjang.

## 15. Tahapan Implementasi

| Tahap | Isi | Hasil |
|---|---|---|
| **M1 — Fondasi** | `go.mod`, config loader + validasi, store JSON (atomic write, mutex), model & aturan bisnis + unit test | `go test` hijau |
| **M2 — Halaman publik** | Beranda, halaman event, form input, tabel + pencarian, CSS responsif | Bisa input & lihat data |
| **M3 — Admin** | Login/logout, sesi, CSRF, buat/ubah event, verifikasi status, hapus, rekap, penanda duplikat, export CSV | Alur verifikasi lengkap |
| **M4 — Hardening & deploy** | Rate limit, security header, log aksi admin, `ci.yml`, `deploy.yml`, panduan setup service & HTTPS di README | Berjalan di self-hosted |

## 16. Risiko & Pertanyaan Terbuka

| # | Item | Default di MVP |
|---|---|---|
| R1 | Spam/entri palsu karena form tanpa login. | Rate limit per IP + admin bisa hapus. Opsi lanjutan: kode akses per event. |
| R2 | Privasi: nama & nominal terlihat publik. | Ditampilkan apa adanya; opsi samarkan nama di versi berikutnya. |
| R3 | Mesin self-hosted mati → web tidak bisa diakses. | Diterima untuk MVP; data aman di disk. |
| R4 | File JSON hilang karena disk rusak. | `.bak` satu generasi; disarankan backup folder data terjadwal. |
| Q1 | Bolehkah tanggal transfer sebelum `start_date`? | Ya, maksimal 31 hari sebelumnya (BR-06). |
| Q2 | Perlukan satu nama mengirim beberapa entri dalam satu event? | Ya, diizinkan (mis. transfer dicicil); duplikat persis hanya diberi penanda. |
| Q3 | Perlukah nominal target per orang (mis. iuran Rp150.000) dan status "kurang/lunas"? | Tidak di MVP; kandidat fitur berikutnya. |

## 17. Ide Pengembangan Berikutnya

- Upload bukti transfer (screenshot).
- Kode akses per event agar form tidak bisa diisi sembarang orang.
- Nominal target per peserta dan rekap "sudah/belum lunas".
- Import CSV mutasi rekening untuk pencocokan otomatis (nominal + tanggal) yang tetap dikonfirmasi admin.
- Opsi menyamarkan nama/nominal di halaman publik.
