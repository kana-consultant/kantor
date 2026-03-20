# KANTOR Activity Tracker Extension

Chrome extension untuk mengirim heartbeat aktivitas kerja ke platform KANTOR.

## Fitur

- Manifest V3 service worker
- Heartbeat tiap 30 detik ke `/api/v1/tracker/heartbeat`
- Consent wajib sebelum tracking aktif
- Offline queue + batch sync saat online kembali
- Popup status tracker
- Options page untuk API URL, token, idle timeout, dan excluded domains

## Cara Load di Chrome

1. Buka `chrome://extensions`
2. Aktifkan `Developer mode`
3. Klik `Load unpacked`
4. Pilih folder `extension/`

## Setup Awal

### Jalur yang disarankan untuk user biasa

1. Pastikan stack KANTOR berjalan:
   - `docker compose up --build -d`
2. Login ke web KANTOR di `http://localhost:3000/login`
3. Buka `http://localhost:3000/operational/tracker`
4. Klik `Download Extension`
5. Extract file ZIP
6. Load extension hasil extract sebagai unpacked di `chrome://extensions`
7. Kembali ke dashboard tracker lalu klik `Hubungkan & Aktifkan`
8. Consent akan aktif dan extension menyimpan konfigurasi browser ini secara otomatis

### Setup manual untuk tim IT / debugging

1. Pastikan stack KANTOR berjalan:
   - `docker compose up --build -d`
2. Login ke web KANTOR di `http://localhost:3000/login`
3. Ambil access token:
   - buka DevTools
   - masuk ke tab `Application`
   - buka `Local Storage`
   - pilih origin `http://localhost:3000`
   - buka key `kantor-auth`
   - salin nilai `state.session.tokens.access_token`
4. Buka popup extension
5. Isi `API URL` dengan `http://localhost:3000/api/v1`
6. Paste access token
7. Klik `Simpan Konfigurasi`
8. Klik `Aktifkan Tracking`

## Alur Uji Manual

1. Setelah tracking aktif, buka domain produktif seperti:
   - `github.com`
   - `docs.google.com`
   - `figma.com`
2. Diamkan tab aktif minimal 30-60 detik agar heartbeat terkirim beberapa kali
3. Buka dashboard web di `http://localhost:3000/operational/tracker`
4. Verifikasi:
   - consent banner hilang
   - `Total Active Time` bertambah
   - `Most Used Domain` tampil
   - `Top Domains` menampilkan domain yang dibuka
   - untuk admin, tab `Team Activity` juga menampilkan tabel `Consent Audit` yang menunjukkan siapa yang sedang on/off tracker
5. Uji idle:
   - diamkan browser sesuai idle timeout
   - popup harus berubah ke status `Idle`
6. Uji excluded domains:
   - tambahkan domain seperti `youtube.com` di settings
   - buka domain itu
   - domain tersebut tidak boleh tampil di dashboard tracker
7. Uji revoke consent:
   - buka settings extension
   - klik `Revoke Consent`
   - tracking harus berhenti
   - backend akan menolak session/heartbeat baru sampai consent diaktifkan lagi

## Catatan Auth

- Extension menyimpan access token di `chrome.storage.local`
- Refresh token tidak disimpan langsung oleh extension
- Saat access token expired, extension akan mencoba `POST /api/v1/auth/refresh` dengan cookie browser yang masih aktif
- Jika refresh gagal, user perlu mengambil access token baru dari web KANTOR

## Catatan Development

- Default API URL untuk dev adalah `http://localhost:3000/api/v1`
- Dashboard web tracker ada di `http://localhost:3000/operational/tracker`
- Domain seperti `chrome://`, `chrome-extension://`, `about:`, `edge:` dan `file:` tidak di-track
