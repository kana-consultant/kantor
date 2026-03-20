# KANTOR Activity Tracker Extension

Chrome extension untuk mengirim heartbeat aktivitas kerja ke platform KANTOR.

## Fitur

- Manifest V3 service worker
- Heartbeat tiap 30 detik ke `/api/v1/tracker/heartbeat`
- Consent wajib sebelum tracking aktif
- Offline queue + batch sync saat koneksi kembali
- Popup status tracker yang fokus pada status, waktu aktif, dan domain aktif
- Options page untuk fallback manual, idle timeout, dan excluded domains

## Cara Pasang untuk User

Flow utama sekarang dilakukan dari web app KANTOR, bukan dari repository.

1. Jalankan stack KANTOR:
   - `docker compose up --build -d`
2. Login ke web KANTOR di `http://localhost:3000/login`
3. Buka `http://localhost:3000/operational/tracker`
4. Klik `Download Extension`
5. Extract file ZIP yang terunduh
6. Buka `chrome://extensions`
7. Aktifkan `Developer mode`
8. Klik `Load unpacked`
9. Pilih folder hasil extract ZIP
10. Kembali ke dashboard tracker KANTOR
11. Klik `Hubungkan & Aktifkan`

Setelah langkah itu, extension akan menerima konfigurasi browser ini dari dashboard web. User biasa tidak perlu paste token secara manual.

## Setup Manual untuk IT / Debugging

Gunakan ini hanya jika auto-connect dari dashboard tidak bisa dipakai.

1. Login ke web KANTOR di `http://localhost:3000/login`
2. Ambil access token dari browser:
   - buka DevTools
   - tab `Application`
   - buka `Local Storage`
   - pilih origin `http://localhost:3000`
   - buka key `kantor-auth`
   - salin `state.session.tokens.access_token`
3. Buka popup extension
4. Klik `Setup manual untuk IT`
5. Isi `API URL` dengan `http://localhost:3000/api/v1`
6. Paste access token
7. Klik `Simpan Konfigurasi Manual`
8. Klik `Aktifkan Tracking`

## Cara Kerja Singkat

1. Dashboard web mengirim konfigurasi ke extension.
2. Extension menyimpan `apiBaseUrl` dan access token di `chrome.storage.local`.
3. Extension memulai session tracker.
4. Setiap 30 detik extension mengirim heartbeat berisi URL aktif, domain, judul halaman, dan status idle.
5. Backend menyimpan data ke `activity_sessions` dan `activity_entries`.
6. Dashboard KANTOR menampilkan agregasi hasil tracking.

Catatan:
- kategori domain yang dianggap benar tetap mengikuti data di dashboard KANTOR
- popup extension sengaja tidak menampilkan badge kategori agar tidak menyesatkan saat domain aktif belum masuk ringkasan lokal popup

## Alur Uji Manual

1. Setelah tracking aktif, buka domain produktif seperti:
   - `github.com`
   - `docs.google.com`
   - `figma.com`
2. Diamkan tab aktif minimal 30-60 detik
3. Buka dashboard web di `http://localhost:3000/operational/tracker`
4. Verifikasi:
   - consent banner hilang
   - `Total Active Time` bertambah
   - `Most Used Domain` tampil
   - `Top Domains` menampilkan domain yang dibuka
   - untuk admin, tab `Team Activity` menampilkan `Consent Audit`
5. Uji idle:
   - diamkan browser sesuai idle timeout
   - popup harus berubah ke status `Idle`
6. Uji excluded domains:
   - tambahkan domain seperti `youtube.com` di settings extension
   - buka domain itu
   - domain tersebut tidak boleh tampil di dashboard tracker
7. Uji revoke consent:
   - buka settings extension
   - klik `Revoke Consent`
   - tracking harus berhenti
   - backend akan menolak session dan heartbeat baru sampai consent diaktifkan lagi

## Catatan Auth

- extension menyimpan access token di `chrome.storage.local`
- refresh token tidak disimpan langsung oleh extension
- saat access token expired, extension akan mencoba `POST /api/v1/auth/refresh` dengan cookie browser yang masih aktif
- jika refresh gagal, user perlu menghubungkan ulang dari dashboard atau melakukan setup manual lagi

## Catatan Development

- default API URL untuk dev adalah `http://localhost:3000/api/v1`
- dashboard web tracker ada di `http://localhost:3000/operational/tracker`
- domain seperti `chrome://`, `chrome-extension://`, `about:`, `edge:`, dan `file:` tidak di-track
- jika Anda mengembangkan extension langsung dari repository, folder yang di-load unpacked adalah folder `extension/`
