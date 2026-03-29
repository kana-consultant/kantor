function padDatePart(value: number) {
  return String(value).padStart(2, "0");
}

export function formatDateInputValue(date: Date = new Date()) {
  return `${date.getFullYear()}-${padDatePart(date.getMonth() + 1)}-${padDatePart(date.getDate())}`;
}

export function extractDateInputValue(value: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    return "";
  }

  const match = trimmed.match(/^(\d{4}-\d{2}-\d{2})/);
  if (match?.[1]) {
    return match[1];
  }

  const parsed = new Date(trimmed);
  if (Number.isNaN(parsed.getTime())) {
    return "";
  }

  return formatDateInputValue(parsed);
}

export function parseCalendarDate(value: string) {
  const dateOnly = extractDateInputValue(value);
  if (!dateOnly) {
    return null;
  }

  const parts = dateOnly.split("-").map((part) => Number(part));
  if (parts.length !== 3 || parts.some((part) => Number.isNaN(part))) {
    return null;
  }

  const year = parts[0] ?? 0;
  const month = parts[1] ?? 1;
  const day = parts[2] ?? 1;
  return new Date(year, month - 1, day);
}

export function toUTCDateOnlyISOString(value: string) {
  const dateOnly = extractDateInputValue(value);
  return dateOnly ? `${dateOnly}T00:00:00Z` : value;
}

export function formatCalendarDate(value: string, locale = "id-ID") {
  const parsed = parseCalendarDate(value);
  if (!parsed) {
    return "-";
  }

  return new Intl.DateTimeFormat(locale).format(parsed);
}

export function getCalendarDayDifference(value: string, baseDate: Date = new Date()) {
  const parsed = parseCalendarDate(value);
  if (!parsed) {
    return null;
  }

  const startOfBaseDate = new Date(baseDate.getFullYear(), baseDate.getMonth(), baseDate.getDate());
  return Math.round((parsed.getTime() - startOfBaseDate.getTime()) / (1000 * 60 * 60 * 24));
}
