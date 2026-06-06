function pad(n: number) { return String(n).padStart(2, '0') }

function parts(d: Date) {
  return {
    dd: pad(d.getDate()),
    mm: pad(d.getMonth() + 1),
    yyyy: d.getFullYear(),
    HH: pad(d.getHours()),
    MM: pad(d.getMinutes()),
    SS: pad(d.getSeconds()),
  }
}

export function fmtDateTime(v: string | Date | null | undefined): string {
  if (!v) return '—'
  const { dd, mm, yyyy, HH, MM, SS } = parts(new Date(v))
  return `${dd}.${mm}.${yyyy} ${HH}:${MM}:${SS}`
}

export function fmtTime(v: string | Date | null | undefined): string {
  if (!v) return '—'
  const { HH, MM, SS } = parts(new Date(v))
  return `${HH}:${MM}:${SS}`
}
