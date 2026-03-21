# RBAC Changes Log

Dokumen ini berisi semua perubahan RBAC yang dilakukan di luar instruksi prompt, untuk direview.

## Permissions yang Ditambahkan

| Permission ID | Module | Deskripsi | Alasan |
|---------------|--------|-----------|--------|
| `hris:bonus:edit` | `hris` | Mengedit bonus yang masih `pending` | Fitur update bonus sudah ada di backend dan frontend, tetapi daftar permission minimum di prompt belum memisahkan edit bonus dari create/approve. |
| `hris:bonus:delete` | `hris` | Menghapus bonus yang masih `pending` | Endpoint delete bonus sudah ada dan merupakan aksi destruktif terpisah, jadi tidak aman digabung ke permission edit. |
| `hris:reimbursement:edit` | `hris` | Mengedit reimbursement dan upload attachment | Flow upload attachment reimbursement sudah ada, tetapi perlu permission mutasi terpisah dari create dan approve agar tidak terlalu lebar. |

## Endpoints yang Sebelumnya Tanpa RBAC

| Method | Path | Permission yang Di-assign | Alasan |
|--------|------|--------------------------|--------|
| `GET` | `/api/v1/modules` | `admin:roles:view` OR `admin:users:view` OR `admin:settings:view` | Endpoint ini dipakai oleh halaman admin untuk dropdown modul. Saya protect di [backend/internal/app/app.go](backend/internal/app/app.go) agar tidak menjadi katalog publik. |
| `ALL` | `/api/v1/admin/*` | `RequireModuleAccess("admin")` + permission spesifik per endpoint | Sebelumnya akses admin hanya bergantung pada super admin/permission lama. Sekarang seluruh group wajib punya akses modul admin juga. |
| `ALL` | `/api/v1/operational/*` | `RequireModuleAccess("operational")` + permission spesifik per endpoint | Menegakkan model baru bahwa permission saja tidak cukup jika user memang tidak di-assign ke modul operasional. |
| `ALL` | `/api/v1/hris/*` | `RequireModuleAccess("hris")` + permission spesifik per endpoint | Menutup akses direct URL/API untuk user yang tidak punya assignment modul HRIS. |
| `ALL` | `/api/v1/marketing/*` | `RequireModuleAccess("marketing")` + permission spesifik per endpoint | Menyamakan backend dengan konsep module-scoped roles baru. |
| `ALL` | `/api/v1/tracker/*` | `RequireModuleAccess("operational")` + permission tracker | Chrome Extension Tracker berada di domain operasional, jadi sekarang ikut module gate juga. |
| `ALL` | `/api/v1/wa/*` | `RequireModuleAccess("operational")` + permission WA | WA Broadcast adalah fitur operasional; sebelumnya belum ditegakkan di level modul. |
| `GET` | `/api/v1/admin/settings/departments` | `admin:settings:view` | Halaman admin settings perlu daftar department untuk `auto_create_employee` tanpa memaksa role settings-only punya akses modul HRIS. |

## Keputusan Permission yang Diambil Sendiri

| File | Endpoint/Fitur | Permission | Alasan & Pertimbangan |
|------|---------------|------------|----------------------|
| `backend/internal/app/app.go` | `GET /api/v1/modules` | `admin:roles:view` OR `admin:users:view` OR `admin:settings:view` | Ketiga halaman admin membutuhkan katalog modul. Saya pilih kombinasi OR agar tidak memaksa semua admin screen punya permission `admin:roles:view`. |
| `backend/internal/handler/hris/reimbursements.go` | `POST /api/v1/hris/reimbursements/{reimbursementID}/attachments` | `hris:reimbursement:edit` | Upload attachment adalah aksi edit terhadap reimbursement existing, bukan create baru dan bukan approve. |
| `backend/internal/app/app.go` | `DELETE /api/v1/hris/bonuses/{bonusID}` | `hris:bonus:delete` | Delete adalah aksi destruktif dan sebaiknya tidak ikut otomatis bersama `hris:bonus:edit`. |
| `backend/internal/service/hris/compensation.go` | Ownership filter bonus | Elevated via `hris:bonus:approve`, selain itu ownership-filtered | Prompt tidak menyediakan permission `bonus:view_all`. Saya pakai `approve` sebagai indikator akses bonus lintas karyawan karena itu milik manager/admin/elevated reviewer. |
| `backend/internal/service/hris/finance.go` | Ownership filter finance records | Elevated via `hris:finance:approve`, selain itu hanya record yang disubmit actor | Prompt menjelaskan reviewer/approver melihat lebih luas, tetapi tidak ada permission `finance:view_all`, jadi `approve` saya gunakan sebagai elevated access. |
| `backend/internal/service/hris/reimbursements.go` | Ownership filter reimbursement list/detail | Elevated via `hris:reimbursement:view_all`, selain itu hanya reimbursement milik employee actor | Ini diselaraskan langsung dengan prompt yang meminta staff hanya melihat reimbursement milik sendiri. |
| `backend/internal/handler/marketing/common.go` | Helper admin campaign column management | `marketing:campaign:manage_columns` | Saya align helper internal marketing ke permission baru berbasis resource campaign, bukan role slug lama atau generic column manage. |
| `backend/internal/handler/operational/projects.go` | Mutasi member project | `operational:project:manage_members` | Mengelola anggota project lebih presisi jika dipisah dari `operational:project:edit`, supaya custom role bisa edit project tanpa wajib boleh ubah member. |
| `backend/internal/service/hris/reimbursements.go` | Notifikasi approval reimbursement | `hris:reimbursement:approve` | Setelah pindah ke custom role per modul, saya ganti fanout notifikasi dari slug role lama ke effective permission agar role custom seperti `finance_officer` tetap menerima event yang relevan. |
| `backend/internal/service/marketing/campaigns.go` | Notifikasi campaign live | `marketing:campaign:edit` | Role custom marketing yang punya hak kelola campaign harus menerima event tanpa dipaksa memakai slug `manager/admin`. |
| `backend/internal/service/marketing/leads.go` | Notifikasi lead won/lost | `marketing:leads:edit` | Saya pakai permission edit lead sebagai indikator operator/pengelola pipeline yang memang perlu notifikasi perubahan outcome. |

## Permission String yang Diubah dari Format Lama

| Lama | Baru | Alasan |
|------|------|--------|
| `operational:kanban:view` | `operational:column:view` dan `operational:task:view` | Resource `kanban` terlalu lebar. Saya pecah mengikuti resource nyata yang diakses endpoint: kolom dan task. |
| `operational:kanban:create` | `operational:column:manage` atau `operational:task:create` | Create kolom dan create task adalah dua aksi berbeda dengan resiko berbeda. |
| `operational:kanban:edit` | `operational:column:manage` atau `operational:task:edit` | Menjaga konsistensi `module:resource:action` dan memisahkan edit kolom dari edit task. |
| `operational:kanban:delete` | `operational:column:manage` atau `operational:task:delete` | Delete task dan delete kolom tidak lagi disatukan di permission kanban generik. |
| `operational:tracker_consent:audit` | `operational:tracker:view_team` | Audit consent tracker adalah bentuk visibilitas tim, tidak perlu resource permission terpisah. |
| `operational:tracker_domain:manage` | `operational:tracker:manage_domains` | Menyelaraskan naming tracker domain ke resource tracker utama. |
| `marketing:column:manage` | `marketing:campaign:manage_columns` | Kolom yang dikelola adalah kolom kanban campaign, jadi resource yang tepat adalah `campaign`. |
| `operational:project:edit` untuk mutasi member | `operational:project:manage_members` | Mengelola member project saya pisahkan dari edit project biasa agar lebih granular. |
| `hris:reimbursement:approve` untuk mark paid | `hris:reimbursement:mark_paid` | Pembayaran reimbursement adalah aksi lanjutan yang lebih spesifik daripada approve/reject. |
| `hris:bonus:edit` untuk delete bonus | `hris:bonus:delete` | Delete bonus perlu permission destruktif terpisah. |
