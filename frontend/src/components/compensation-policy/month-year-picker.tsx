import type { SalarySafetyPeriod } from "@/services/compensation-policy";

const monthLabels = [
  "Januari",
  "Februari",
  "Maret",
  "April",
  "Mei",
  "Juni",
  "Juli",
  "Agustus",
  "September",
  "Oktober",
  "November",
  "Desember",
];

type MonthYearPickerProps = {
  value: SalarySafetyPeriod;
  /**
   * The current period in the policy timezone (from the server). Months after
   * it cannot be picked: a period that has not started has nothing to
   * evaluate.
   */
  current: SalarySafetyPeriod;
  onChange: (period: SalarySafetyPeriod) => void;
  /** How many years before the current one can be picked (default 1). */
  yearsBack?: number;
};

/** Month + year selects, limited to the current period and earlier. */
export function MonthYearPicker({ value, current, onChange, yearsBack = 1 }: MonthYearPickerProps) {
  const yearOptions = Array.from({ length: Math.max(0, yearsBack) + 1 }, (_, index) => current.year - index);
  // A deep link to an older period must not show another year in the select.
  if (value.year <= current.year && !yearOptions.includes(value.year)) {
    yearOptions.push(value.year);
  }
  const isFutureMonth = (candidateYear: number, candidateMonth: number) =>
    candidateYear > current.year ||
    (candidateYear === current.year && candidateMonth > current.month);

  return (
    <div className="flex gap-2">
      <select
        aria-label="Bulan"
        className="h-9 rounded-sm border-[1.5px] border-transparent bg-surface-muted px-2 text-[13px] text-text-primary outline-none focus:border-[#4C9AFF] focus:bg-surface"
        onChange={(event) =>
          onChange({ year: value.year, month: Number(event.target.value) })
        }
        value={value.month}
      >
        {monthLabels.map((label, index) => (
          <option
            disabled={isFutureMonth(value.year, index + 1)}
            key={label}
            value={index + 1}
          >
            {label}
          </option>
        ))}
      </select>
      <select
        aria-label="Tahun"
        className="h-9 rounded-sm border-[1.5px] border-transparent bg-surface-muted px-2 text-[13px] text-text-primary outline-none focus:border-[#4C9AFF] focus:bg-surface"
        onChange={(event) => {
          const nextYear = Number(event.target.value);
          // Switching back to the current year may leave a future month
          // selected; clamp it to the current month.
          onChange({
            year: nextYear,
            month: isFutureMonth(nextYear, value.month) ? current.month : value.month,
          });
        }}
        value={value.year}
      >
        {yearOptions.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    </div>
  );
}
