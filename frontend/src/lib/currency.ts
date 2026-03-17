export function formatIDR(value: number) {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    maximumFractionDigits: 0,
  }).format(value);
}

export function formatRupiahInput(value: number) {
  if (!Number.isFinite(value) || value <= 0) {
    return "";
  }

  return new Intl.NumberFormat("id-ID", {
    maximumFractionDigits: 0,
  }).format(Math.trunc(value));
}

export function parseRupiahInput(raw: string) {
  const digits = raw.replace(/[^\d]/g, "");
  if (!digits) {
    return 0;
  }

  return Number(digits);
}

export function parseLooseCurrency(raw: string) {
  const digits = (raw ?? "").replace(/[^\d-]/g, "");
  if (!digits || digits === "-") {
    return 0;
  }

  const value = Number(digits);
  return Number.isFinite(value) ? value : 0;
}
