// Client-side NIK (Nomor Induk Kependudukan) checks. They mirror the backend
// (service/hris/hr_profiles.go: isNIKFormat / ValidateNIK) so HR sees the
// problem before saving; the backend stays the authority.
//
// Layout: digits 1-6 region, 7-8 birth day (+40 for women), 9-10 birth month,
// 11-12 last two digits of the birth year, 13-16 serial (never 0000).

import type { HRGender } from "@/types/hris";

/** Removes every whitespace character, like the backend does before checking. */
export function normalizeNIK(value: string): string {
  return value.replace(/\s+/g, "");
}

/** Returns an error message when the NIK is not a well-formed 16-digit NIK. */
export function nikFormatError(nik: string): string | null {
  if (!/^\d{16}$/.test(nik)) {
    return "NIK harus 16 digit angka";
  }
  if (nik.slice(0, 6) === "000000") {
    return "Kode wilayah NIK (digit 1-6) tidak boleh 000000";
  }
  if (nik.slice(12) === "0000") {
    return "Nomor urut NIK (digit 13-16) tidak boleh 0000";
  }
  return null;
}

/** Digits 7-12 the NIK must carry for this birth date ("YYYY-MM-DD") and gender. */
export function expectedNIKBirthDigits(birthDate: string, gender: HRGender): string | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(birthDate);
  if (!match) {
    return null;
  }
  const [, year = "", month = "", day = ""] = match;
  const dayPart = Number(day) + (gender === "female" ? 40 : 0);
  return `${String(dayPart).padStart(2, "0")}${month}${year.slice(2)}`;
}

/** Returns an error message when digits 7-12 do not match the birth date and gender. */
export function nikBirthDataError(nik: string, birthDate: string, gender: HRGender): string | null {
  const expected = expectedNIKBirthDigits(birthDate, gender);
  if (!expected) {
    return null;
  }
  if (nik.slice(6, 12) !== expected) {
    return `Digit 7-12 NIK harus ${expected} untuk tanggal lahir dan jenis kelamin ini (tanggal lahir${
      gender === "female" ? " + 40 untuk perempuan" : ""
    }, bulan, dua digit tahun)`;
  }
  return null;
}
