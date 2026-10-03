# HRIS Documents: Payslips and Employment Contracts

KANTOR generates two kinds of HR documents from the employee data it already holds:

- **Slip Gaji** (monthly payslip): one per employee and month, built from the salary
  record, approved bonuses, paid reimbursements and manual adjustments.
- **Kontrak Kerja**: a fixed-term employment agreement (**PKWT**) together with its
  **NDA & IP assignment (NDA/HKI)**, generated and numbered as a pair. Permanent (PKWTT)
  and internship (Magang) contracts can be recorded without documents.

Documents are rendered from the DOCX templates embedded in the backend
(`backend/internal/docgen/templates/`), converted to PDF by LibreOffice inside the backend
process, stored encrypted, and emailed to the employee through a dedicated Gmail account.

> Operators: installation (LibreOffice, fonts, Gmail account, outbound SMTP, key handling)
> is in the [Deployment Guide](deployment.md#9-document-engine-pdf).

**Running it locally needs no extra `.env` keys.** Install LibreOffice (macOS:
`brew install --cask libreoffice`) and restart the backend: it is detected automatically and
its profile goes to your user cache dir. With `APP_ENV=development`, document email is
captured by Mailpit at `localhost:1025` (UI `http://localhost:8025`) instead of going to
Gmail, so run Mailpit too; it is not in the committed `docker-compose.yml`, start it with
`docker run -d --name mailpit -p 127.0.0.1:1025:1025 -p 127.0.0.1:8025:8025 axllent/mailpit`.
`SOFFICE_BIN`, `DOCUMENT_LO_PROFILE_DIR` and
`DOCUMENT_MAIL_DEV_SMTP_ADDR` are optional overrides only (`off` switches PDF or the capture
off); see the [environment variable reference](deployment.md#environment-variable-reference).
Admin > Settings > **Email Dokumen** shows the effective status:
"Konversi PDF: aktif (LibreOffice terdeteksi otomatis)" or why it is off, and a "Mode
development" banner while email is captured. NixOS and the Docker image keep setting these
values explicitly.

**LibreOffice version and page count.** Production (NixOS) renders with LibreOffice 25.x and
the Liberation fonts. Older LibreOffice releases space lines more loosely, so the same
contract can have a different page count locally: the sample PKWT is about 4 pages with
LibreOffice 25.x and 5 with 7.6, and a payslip with many lines can spill onto a second page
with 7.6 while it still fits one page with 25.x. For local previews that match production,
use a current LibreOffice (macOS: `brew upgrade --cask libreoffice`).

## 1. Admin setup (in this order)

1. **Company profile** — Admin > Settings > **Profil Perusahaan**: legal name (PT), address,
   business type, city (place of signing), signer name and title, HR contact email, document
   code (for example `CTN`), payday (day of month) and annual leave days. Upload the company
   **logo** there too: it is trimmed and fitted into the payslip's logo slot. Without a
   logo the slot stays blank (the templates carry no branding of their own).
2. **Document email** — Admin > Settings > **Email Dokumen**: enable it, enter the
   send-only Gmail / Workspace address, sender name, port (587 STARTTLS or 465) and the
   Google **app password**, then press **Kirim email uji**. The password is stored
   encrypted and never shown again. Changing the address or port requires entering a new
   app password in the same save: the stored one is never reused for another account or
   port. The SMTP host
   is always `smtp.gmail.com` (in development, email is captured by Mailpit instead; the
   card says so). The card also shows whether PDF conversion is available.
3. **HR role** — Admin > Roles: create a custom role for HR (for example "HR") in the HRIS
   module and assign it to the HR staff. The eight document permissions are sensitive:
   the built-in roles give them to Admin only, never to Manager or Staff.

   | Permission | Allows |
   |------------|--------|
   | `hris:employee_identity:view` | See identity data (masked NIK, birth data, KTP address), the full bank account number, personal email; needed (with the contract permissions) to download contract PDFs |
   | `hris:employee_identity:edit` | Fill or replace identity data and the personal email |
   | `hris:contract:view` | List and open contracts |
   | `hris:contract:manage` | Create, edit, generate, renew, change status; DOCX download |
   | `hris:contract:send` | Email contracts |
   | `hris:payslip:view` | List and open payslips, preview PDFs |
   | `hris:payslip:manage` | Generate drafts, edit notes and manual lines, void & reissue; DOCX download |
   | `hris:payslip:send` | Email payslips |

   HR usually also needs `hris:employee:view`/`edit`, `hris:salary:view`/`create`,
   `hris:bonus:view` and `hris:reimbursement:view_all`. Combined gates:
   - every payslip endpoint needs `hris:payslip:<x>` **and** `hris:salary:view`;
   - contract compensation is shown and editable only with `hris:salary:view`;
   - the PKWT/NDA PDF needs `hris:contract:view` **and** `hris:employee_identity:view`
     (the PKWT contains the full NIK and bank account number); DOCX needs
     `hris:contract:manage` **and** `hris:employee_identity:view`;
   - the **PKWT** part (PDF and DOCX) also needs `hris:salary:view`, because it prints the
     compensation; the NDA part does not.

   Employees do not see their own documents in the app in this version; they receive them
   by email.

## 2. Payslips (Slip Gaji): month-end flow

1. Around the 20th, HR opens **HRIS > Slip Gaji** and picks the month. Every active or
   probation employee who joined on or before the last day of the month is listed, with
   warnings: no salary record (blocks generation), base salary below Rp100.000, joined or
   left during the month (check pro-rata), a bonus of the month still pending, an approved
   reimbursement not yet paid, missing job title / employee code / bank / email.
2. HR fixes what is needed (salary record, job title in the employee profile, asks Finance to
   approve or pay), selects the employees and clicks **Generate Draft**. The pay date
   defaults to the company payday, moved to the previous weekday when it falls on a weekend.
   Drafts appear at once; PDFs render in the background (10-20 s for the first one).
3. HR previews each slip. A draft can get a one-line note (max 120 characters) and up to five
   manual lines (positive = earning, negative = deduction, e.g. "Penyesuaian pro-rata").
   Regenerating a draft keeps them.
4. HR selects the reviewed rows and clicks **Kirim Terpilih**. The confirmation lists every
   recipient and flags a new address ("alamat baru"); slips already sent are skipped. Each
   email carries only the PDF and a neutral bilingual body without amounts.
5. A wrong slip that was already sent is never edited: **Batalkan & Terbitkan Ulang** voids
   it (kept for the record) and creates a new draft numbered `<number>-R1`, whose email says
   which slip it replaces. The reissue keeps exactly the bonuses and reimbursements of the
   voided slip.

**Content.** Earnings: base salary, each allowance, each approved bonus, positive manual
lines. Deductions (separate table): each salary deduction and negative manual lines. Paid
reimbursements in their own table. Total received = earnings − deductions +
reimbursements, also written out in words. The bank account is shown masked
(`******7890`). Job title and employment status come from the active contract, else from
the employee profile.

### Carry-over rule (bonuses and reimbursements)

A slip is a snapshot. Items that arrive after a slip was sent go on the **next** slip, and
nothing from before the first slip is swept in. With M the slip's month and L the
employee's latest **sent** (not void) slip of an earlier month:

- **Bonuses**: approved, not on any sent slip, period not after M, and
  - period = M, or
  - L exists and the bonus period is after L's period (L missed it), or
  - L exists and the bonus was approved after L was generated (late approval).

  Without L (the employee's first slip) only bonuses of period M count, so the first slip
  after go-live does not collect every historical bonus.
- **Reimbursements**: paid, not on any sent slip, paid before generation, and paid on or
  after the floor: the first day of M (00:00 WIB) without L, else the time L was generated.
  The payment date counts, not the transaction date: a receipt from last month paid this
  month is on this month's slip.

A draft of an earlier month reserves its items, so the same bonus is never on two slips.

## 3. Employment contracts (Kontrak Kerja)

1. **HRIS > Kontrak Kerja > Buat Kontrak**, pick the employee. The employee record should
   already use their future work email (documents go to the login email).
2. The **Data Legal** panel lists what is missing. HR with identity permissions fills NIK,
   birth place and date, gender, account holder name and KTP address inline (the NIK is
   checked against birth date and gender, then shown masked for good). Missing bank account
   or phone are fixed inline on the employee record.
3. Fill the terms: start and end date, job title, department, direct supervisor, location,
   work mode (WFO / hybrid / remote + detail), job description and the PKWT basis (each a
   single Indonesian sentence), working days and hours, notice period, compensation
   (with `hris:salary:view`, prefilled from the salary record but never written back to it),
   benefits, and the NDA values (incident report hours, non-solicit months,
   confidentiality years, prior works).
4. **Generate** first runs the preflight (missing data, warnings: employee still on
   probation, renewal chain above five years, a duration that is not whole months), then
   assigns the numbers and renders both PDFs.
5. Preview both, then **Kirim via Email**: one email with the two PDFs to the login email,
   or the personal email for a new hire whose work mailbox does not exist yet. CC is limited
   to addresses on the domain of the company HR contact email.
6. **Revisi**: editing a sent, unsigned contract returns it to draft with revision + 1; the
   numbers stay, and the next email subject says "(Revisi 1)".
7. **Tandai Ditandatangani** records the signing date; a signed contract can no longer be
   edited. From `notice_days + 14` days before the end date the list shows the notice
   deadline. **Perpanjang** clones the terms into a linked draft starting the day after
   the end date; **Akhiri** ends it (its checklist reminds HR of the compensation payment
   and handover); **Batalkan** cancels it.

**Record only** ("Catat saja"): PKWTT and Magang are always record-only, and a PKWT signed on
paper can be recorded the same way. No document is generated; the record still feeds the
payslip (employment status, job title) and the PKWT five-year check.

## 4. Numbering

| Document | Format | Example |
|----------|--------|---------|
| PKWT | `<NNN>/PKWT/<doc code>/<month in Roman numerals>/<year>` | `001/PKWT/CTN/X/2026` |
| NDA/HKI | `<NNN>/NDA-HKI/<doc code>/<month>/<year>` (same NNN as its PKWT) | `001/NDA-HKI/CTN/X/2026` |
| Payslip | `PAY/<year>/<month>/<employee number, 4 digits>` | `PAY/2026/09/0002` |
| Reissued payslip | `<payslip number>-R<n>` | `PAY/2026/09/0002-R1` |
| Employee code | `<doc code>-<employee number, 4 digits>` (or the 4 digits alone without a doc code) | `CTN-0002` |

- The contract counter is per tenant and per month of the document date, shared by the
  PKWT/NDA pair, assigned once at the first generate; revisions keep the numbers.
- Employee numbers are assigned once, at the employee's first payslip or contract, and
  never reused.

## 5. Legal notes

- The templates are **drafts** prepared for this product. Have them reviewed by your own
  legal counsel before using them with real employees, and again when regulations change.
  KANTOR does not give legal advice.
- PKWT (Indonesian fixed-term employment, UU 13/2003 as amended by UU 6/2023 and PP 35/2021),
  as reflected in the warnings:
  - a PKWT has a fixed end date and **no probation period**; the preflight warns when the
    employee is still marked as probation;
  - the total duration of a PKWT including extensions may not exceed **five years**; the
    preflight sums the renewal chain;
  - at the end of a PKWT the employee is owed **uang kompensasi** (pay it as a manual line on
    the final payslip);
  - the agreement is bilingual; the Indonesian text prevails, which is why the free-text
    terms are entered in Indonesian.
- Contracts are signed outside KANTOR (on paper or with an e-signature tool); KANTOR records
  the signing date.

## 6. Data protection

- **Identity data** (NIK, birth place and date, gender, account holder name, KTP address) is
  stored as one encrypted blob (AES-256-GCM, `DATA_ENCRYPTION_KEY`). After saving, the NIK
  is shown only masked (`327301**********`); no NPWP is collected. Every identity read is
  access-logged.
- **Bank account numbers** are stored encrypted (`employees.bank_account_encrypted`) and
  masked in every employee response, export and MCP tool unless the caller has
  `hris:employee_identity:view` or it is their own record. A masked value sent back by a
  form is ignored, so it never overwrites the real number. The full number appears only in
  the PKWT. This release keeps the old plaintext column too and writes both, so the upgrade
  changes no existing value; clearing the plaintext is a separate opt-in
  (`BANK_ACCOUNT_CLEAR_PLAINTEXT=true`) to run only after a verified backup
  ([details](deployment.md#11-data-protection-at-rest)).
- **Snapshots**: payslips and contracts store their content encrypted at generation time;
  later changes to salaries, bonuses or the employee record do not change a generated
  document. Payslips and contracts keep the employee record from being deleted (set the
  employee to resigned/terminated instead).
- **Files**: only PDFs are stored, encrypted, under `UPLOADS_DIR/documents/<tenant>/`; DOCX
  files are rendered on demand. Document responses are sent with `Cache-Control: no-store`.
  Temporary plaintext files of the converter are deleted after each job.
- **Email**: documents go to the login email of the linked user (only the user can change it,
  with their password), not to the editable employee email; the personal email is used only
  when HR picks it. The delivery log keeps recipient, subject, attachment names and SHA-256,
  not the message. Gmail keeps a copy in Sent Mail: restrict access to the sending account
  and set a retention rule there.
- **Audit**: generate, download, send, void, revision and status changes are audited with
  metadata only (document number, period, format, masked recipient). The audit writer
  additionally replaces the values of `bank_account_number`, `nik`, `identity`,
  `base_salary`, `allowances`, `deductions`, `amount`, `net_salary`, passwords and API keys
  with `"[redacted]"` for every new audit entry; employee updates add `changed_fields`
  (names only) so a changed bank account number is still visible as a change. Entries
  written before this release are left as they are unless the opt-in
  `AUDIT_SCRUB_EXISTING=true` is run after a verified backup
  ([details](deployment.md#audit-log-redaction)).
- **MCP**: the document, HR profile, email-delivery and mail/company settings routes are not
  exposed as MCP tools.
- **Retention**: generated PDFs and the delivery log are kept until deleted; there is no
  automatic purge. Keep them as long as your payroll and employment record policy requires.
- **Encryption key**: do not rotate `DATA_ENCRYPTION_KEY` until a re-encryption tool exists
  ([why](deployment.md#encryption-key-rotation)). If it was ever rotated,
  `DATA_ENCRYPTION_KEY_PREVIOUS` must stay set permanently: key versions are positional, so
  dropping it makes both the old data and everything written since the rotation unreadable.

## 7. What existing users notice on day one

These changes apply as soon as this release is deployed, with or without documents being
generated:

- **Bank account numbers are masked** (`******7890`) in employee pages, exports and MCP
  tools for every role without `hris:employee_identity:view`, except on the user's own
  record. The built-in roles give that permission to Admin only.
- **A linked employee's email is locked** in the HR form: it is the login address of the
  user account, and only that user can change it (Profile, with their password).
- **Employees with a payslip or contract cannot be deleted**; set them to
  resigned/terminated instead.
- **New salary-safety rule** (HRIS > Status Keamanan Gaji): the monthly hour target is
  pro-rated by the weekdays elapsed (through yesterday, or the month end for past months)
  since the 1st or the join date, and an employee is safe when the hours tracked in the
  month reach it. Days below the daily minimum and absent days are warnings only (before,
  any such day made the employee at risk). Part Time, Internship, Project Based and
  Outsourcing show "no data" (no target yet), and employees who have not started yet are
  not listed.
- **New audit entries show `"[redacted]"`** for amounts, account numbers, NIK and secrets.
  Entries written before the upgrade keep their values; they change only if an admin runs
  the opt-in scrub after a backup ([details](deployment.md#audit-log-redaction)).
- **MCP**: the tools for `/admin/settings/mail-delivery` (and the document, HR profile,
  company profile and email-delivery routes) are no longer exposed; AI clients that used
  them must use the web app.
