# UI/UX Fixes Checklist

Dokumen ini merangkum revisi UI/UX yang sudah dikerjakan sampai batch terakhir, supaya bisa dicek satu per satu bersama masukan senior.

Status dokumen:
- `[x]` sudah dikerjakan di codebase lokal saat ini
- `[ ]` belum dikerjakan atau belum masuk scope batch ini

## 1. Konsistensi Bahasa

- [x] Rapikan typo dan campur bahasa di auth flow dasar.
- [x] Login page tidak lagi menampilkan typo seperti `dashboaRd`.
- [x] Validation/auth error utama di login/register/profile dibuat lebih konsisten ke Bahasa Indonesia.
- [x] Error screen global dirapikan agar tidak terlalu campur bahasa.
- [x] Beberapa judul/card utama di Operational dirapikan ke Bahasa Indonesia.

## 2. Login / Register Flow

- [x] Setelah register tidak lagi auto-login delayed yang membingungkan.
- [x] Setelah register user diarahkan ke halaman login.
- [x] Setelah login user langsung masuk ke route app tanpa perlu refresh manual.
- [x] Noise refresh token saat login/register dikurangi.

## 3. Shared Shell / Layout

- [x] Dropdown notifikasi tidak lagi transparan.
- [x] Dropdown profile tidak lagi transparan.
- [x] Panel notifikasi/profile di mobile dibuat lebih solid dan lebih usable.
- [x] Sidebar mobile diubah jadi drawer yang lebih proper.
- [x] Scroll sidebar dan page dipisah, tidak lagi saling “menarik” tinggi.
- [x] Topbar sticky di mobile diberi jarak atas agar tidak terlalu mepet.
- [x] Spacing navbar dan content dilonggarkan agar tidak terlalu rapat.
- [x] Hover state button di dark mode diperjelas.

## 4. Modal / Dialog Behavior

- [x] Dialog kecil tidak lagi selalu stretch/full-screen di mobile.
- [x] Delete/confirm dialog tidak lagi punya area kosong aneh di tengah.
- [x] Delete modal sekarang tampil di layer yang benar di atas edit modal/task modal.
- [x] Modal kecil lebih center dan proporsional di mobile.
- [x] Form modal sekarang pakai body yang benar-benar scrollable.
- [x] Footer modal tidak lagi mudah ketutup body saat konten memanjang.

## 5. DataTable / Mobile Card

- [x] Card mobile di DataTable diubah agar label di kiri dan value di kanan.
- [x] Layout card project list di mobile tidak lagi terasa terlalu kosong di kanan.
- [x] Dropdown export memakai z-index tinggi agar tidak kepotong container.

## 6. Profile / Password

- [x] Profile page mobile dirapikan, tombol tidak lagi terasa dobel dan berantakan.
- [x] Fitur ganti password ditambahkan.
- [x] Jika password saat ini salah, sekarang tampil error dan tidak auto-logout.

## 7. Employee / HRIS Profile Enrichment

- [x] Form employee `Role` tidak lagi free text.
- [x] Opsi role employee dibatasi jadi:
  - `Full Time`
  - `Part Time`
  - `Internship`
  - `Project Based`
- [x] Fallback value lama seperti `Belum Ditentukan` dihilangkan dari dropdown.
- [x] Bug form department yang membawa value create sebelumnya sudah diperbaiki.
- [x] Field profile tambahan ditambahkan:
  - `Nomor Rekening`
  - `Bank / E-Wallet`
  - `LinkedIn Profile`
  - `SSH Keys`
- [x] Field tambahan tadi ada di `Edit employee`.
- [x] Field tambahan tadi juga ada di `Profil Saya`.

## 8. Employee Avatar

- [x] Avatar employee diubah dari input URL menjadi upload file gambar.
- [x] Avatar tersimpan ke uploads storage yang dipakai Docker.
- [x] Preview avatar saat edit employee sudah tampil.
- [x] Avatar gambar dipakai di area utama UI, tidak lagi bergantung ke placeholder huruf jika file ada.
- [x] Sync avatar employee ke `users.avatar_url` dirapikan agar tampil lintas modul.

## 9. Employee Detail / Salary

- [x] Noise 404 salary/current yang tidak perlu dikurangi di employee detail.
- [x] Runtime error `isLoading is not defined` di employee detail sudah diperbaiki.
- [x] Klik tab `Salary` di halaman detail employee tidak lagi meledak karena bug props.

## 10. Subscription UX

- [x] Modal `New subscription` sekarang bisa discroll di mobile.
- [x] Form subscription sekarang reset dengan benar saat dibuka ulang.
- [x] Select utama di form subscription sudah diganti ke custom select.

## 11. Campaign UX

- [x] Modal `New campaign` sekarang bisa discroll sampai bawah di mobile.
- [x] Form campaign reset dengan benar saat modal dibuka lagi.
- [x] Select di form campaign sudah diganti ke custom select.
- [x] Filter utama di halaman campaigns sudah diganti ke custom select.

## 12. WA Broadcast

- [x] Form/template modal WA Broadcast dirapikan untuk mobile.
- [x] Preview template WA sekarang punya close yang jelas.
- [x] Preview template WA bisa ditutup dengan lebih normal.
- [x] Action bar/filter template WA Broadcast di mobile dibuat lebih rapi.
- [x] Select di area template/schedule/log filter WA Broadcast yang utama diganti ke custom select.

## 13. Operational Projects List / Overview

- [x] Copy/title utama di project list dan overview Operational dirapikan.
- [x] Filter utama di project list sudah diganti ke custom select.
- [x] Export dropdown aman dan tidak kepotong.

## 14. Project Workspace / Board / Settings

- [x] `Team pulse` dihapus dari project board/settings karena tidak memberi value.
- [x] Layout project workspace dirapikan supaya lebih waras di mobile.
- [x] `People on this board` dan card member dibuat tahan nama/email/role panjang.
- [x] Project board/settings pada mobile dibuat lebih usable dibanding sebelumnya.
- [x] Reorder column/card di kanban board diperbaiki.
- [x] Reorder task/card dibuat lebih stabil lewat drag handle.
- [x] Delete modal task/project tampil di layer yang benar.
- [x] Create project sekarang bisa pilih member sekaligus atur role per member.
- [x] Di project settings, role member bisa diedit inline.
- [x] Footer `Buat project` di modal create project tidak lagi gampang ketutup saat member bertambah.
- [x] Select role member di create project dan project settings sudah custom.

## 15. Notifications / Topbar

- [x] Dropdown notifikasi di mobile dibuat lebih responsif.
- [x] Dropdown profile di mobile dibuat lebih responsif.
- [x] Panel notifikasi/profile punya close yang lebih jelas.

## 16. Dark Mode

- [x] Badge priority `High` diperbaiki agar tetap terbaca di dark mode.
- [x] Beberapa tone warna shell/button/card sudah dipoles agar dark mode lebih konsisten.

## 17. Custom Dropdown / Select

- [x] Komponen custom select baru ditambahkan.
- [x] Tidak lagi bergantung sepenuhnya pada native browser select di area paling terlihat.
- [x] Select custom saat ini sudah dipakai di:
  - Employee form
  - Subscription form
  - Campaign form
  - Project form
  - Project settings member role
  - Projects list filters
  - Campaigns filters
  - WA Broadcast filters / template / schedule utama

## 18. Hal Yang Perlu Dicek Manual Sekarang

Checklist verifikasi cepat:

- [ ] Login page
- [ ] Register page
- [ ] Profile page
- [ ] Dropdown profile + notifikasi di mobile
- [ ] Sidebar mobile
- [ ] `/operational/overview`
- [ ] `/operational/projects`
- [ ] `/operational/projects/:id?view=board`
- [ ] `/operational/projects/:id?view=settings`
- [ ] Create project modal
- [ ] `/operational/wa-broadcast`
- [ ] Template dialog WA Broadcast
- [ ] Schedule dialog WA Broadcast
- [ ] `/hris/employees`
- [ ] Employee detail
- [ ] Edit employee modal
- [ ] Tab salary di employee detail
- [ ] `/hris/subscriptions`
- [ ] Subscription modal
- [ ] `/marketing/campaigns`
- [ ] Campaign modal

## 19. Catatan

- Dokumen ini hanya merangkum yang **sudah dikerjakan** sampai batch saat ini.
- Kalau nanti ada batch revisi baru dari senior, lanjutannya bisa ditambahkan di bawah dokumen ini.
- Dokumen ini belum saya commit kecuali Anda minta.
