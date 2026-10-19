export const MAX_CATALOG_INSTANT = 8_640_000_000_000_000
export function localScheduleInput(instant: number | null): string {
  if (instant === null) return ''
  if (!Number.isSafeInteger(instant) || instant < 0 || instant > MAX_CATALOG_INSTANT) throw new Error('schedule')
  const d = new Date(instant)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${String(d.getFullYear()).padStart(4, '0')}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.${String(d.getMilliseconds()).padStart(3, '0')}`
}
export function parseLocalSchedule(value: string): number | null {
  if (value === '') return null
  const instant = new Date(value).getTime()
  if (!Number.isSafeInteger(instant) || instant < 0 || instant > MAX_CATALOG_INSTANT || localScheduleInput(instant) !== (value.length === 16 ? `${value}:00.000` : value.length === 19 ? `${value}.000` : value)) throw new Error('schedule')
  return instant
}
export function parseInterval(value: string): number | null {
  if (value === '') return null
  if (!/^\d+$/.test(value)) throw new Error('schedule')
  const n = Number(value)
  if (!Number.isSafeInteger(n) || n < 1 || n > MAX_CATALOG_INSTANT) throw new Error('schedule')
  return n
}
export function scheduleText(value: number | null, lang: string): string { return value === null ? '—' : new Intl.DateTimeFormat(lang, { dateStyle: 'medium', timeStyle: 'long' }).format(new Date(value)) }
