const faDigits = ['۰', '۱', '۲', '۳', '۴', '۵', '۶', '۷', '۸', '۹']

export function toFaDigits(value: string | number): string {
  return String(value).replace(/\d/g, (d) => faDigits[Number(d)])
}

/** Formats Rial integer as Persian Toman, e.g. 120000 → «۱۲,۰۰۰ تومان» */
export function formatRial(rials: number): string {
  const tomans = Math.round(rials / 10)
  const fa = toFaDigits(tomans.toLocaleString('en-US'))
  return `${fa} تومان`
}

export function formatDateFa(iso: string): string {
  try {
    return new Intl.DateTimeFormat('fa-IR', {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(new Date(iso))
  } catch {
    return iso
  }
}

export function buildQuery(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') q.set(k, String(v))
  }
  const s = q.toString()
  return s ? `?${s}` : ''
}
