/**
 * The bilingual player-facing dictionary (D-48).
 *
 * EN is the default and VI is the toggle; every user-visible string on the
 * player route comes from `COPY` — no copy is invented at a call site. The
 * wording is the approved table in 02-UI-SPEC.md §Copywriting Contract: adding
 * a language or a key is a deliberate edit here, and the `Copy` shape makes a
 * missing translation a compile error under `strict`.
 *
 * This module also owns the language context and the `auctionlab.lang` storage
 * key. It is plain TypeScript (no JSX) because the UI-SPEC's file list puts the
 * dictionary in `lib/`; the provider therefore builds its element with
 * `createElement` rather than with a `.tsx` file.
 */

import { createContext, createElement, useContext, type ReactNode } from 'react'

export type Lang = 'en' | 'vi'

/** The two segments, in the order the UI-SPEC declares them. */
export const LANGS: readonly Lang[] = ['vi', 'en']

/** EN is the default: there is no browser-language detection (UI-SPEC). */
export const DEFAULT_LANG: Lang = 'en'

/** UI-SPEC §Resume, supersede and storage. */
export const LANG_STORAGE_KEY = 'auctionlab.lang'

type Copy = { readonly en: string; readonly vi: string }

/**
 * Every approved string, keyed by language. `lobby.count` is the one key that
 * is a function of the player count rather than a constant, so it lives in
 * `playerCount` below.
 */
export const COPY = {
  h1: { en: 'Player', vi: 'Người chơi' },
  'cta.join': { en: 'Join', vi: 'Tham gia' },
  'cta.joining': { en: 'Joining…', vi: 'Đang vào…' },
  'form.nicknameLabel': { en: 'Your nickname', vi: 'Nickname của bạn' },
  'form.nicknameHint': {
    en: 'Up to 20 characters. No account needed.',
    vi: 'Tối đa 20 ký tự. Không cần tài khoản.',
  },
  'form.nicknamePlaceholder': { en: 'e.g. An', vi: 'Ví dụ: An' },
  'session.restoring': { en: 'Restoring your session…', vi: 'Đang khôi phục phiên…' },
  'lobby.heading': { en: 'Lobby', vi: 'Phòng chờ' },
  'lobby.budgetLabel': { en: 'Your budget', vi: 'Ngân sách của bạn' },
  'lobby.budgetCaption': { en: 'VND', vi: 'VND' },
  'lobby.you': { en: 'you', vi: 'bạn' },
  'lobby.waiting': {
    en: 'Waiting for other players to join…',
    vi: 'Chờ người chơi khác vào phòng…',
  },
  'presence.connected': { en: 'connected', vi: 'đang kết nối' },
  'presence.disconnected': { en: 'disconnected', vi: 'mất kết nối' },
  'status.connecting': { en: 'Connecting…', vi: 'Đang kết nối…' },
  'status.open': { en: 'Connected', vi: 'Đã kết nối' },
  'status.lost': { en: 'Disconnected', vi: 'Mất kết nối' },
  'status.superseded': { en: 'Session opened elsewhere', vi: 'Phiên đã mở ở nơi khác' },
  'error.nicknameTaken': {
    en: 'That nickname is taken — pick another one.',
    vi: 'Nickname đã có người dùng — chọn tên khác.',
  },
  'error.nicknameInvalid': {
    en: 'Invalid nickname — use 1 to 20 characters.',
    vi: 'Nickname không hợp lệ — cần từ 1 đến 20 ký tự.',
  },
  'error.sessionExpired': {
    en: 'Your old session expired — enter a nickname to rejoin.',
    vi: 'Phiên cũ không còn hiệu lực — nhập nickname để vào lại.',
  },
  'error.generic': { en: 'Something went wrong — try again.', vi: 'Có lỗi xảy ra — thử lại.' },
  'lang.groupLabel': { en: 'Language', vi: 'Ngôn ngữ' },
  'lang.vi': { en: 'Tiếng Việt', vi: 'Tiếng Việt' },
  'lang.en': { en: 'English', vi: 'English' },
} as const satisfies Record<string, Copy>

export type CopyKey = keyof typeof COPY

/** One string in one language. */
export function translate(_lang: Lang, key: CopyKey): string {
  // RED sentinel: the dictionary is declared, the language lookup is not —
  // every key resolves to its English column until the GREEN commit.
  return COPY[key].en
}

/**
 * `lobby.count`: EN has one plural rule, VI has none.
 */
export function playerCount(_lang: Lang, n: number): string {
  // RED sentinel — see translate.
  return `${n} players`
}

/**
 * Read the stored language. A throwing or unavailable storage degrades to the
 * default with no error copy shown.
 */
export function readStoredLang(): Lang {
  // RED sentinel: the storage read is not implemented; the default is.
  return DEFAULT_LANG
}

/** Persist the language choice. */
export function writeStoredLang(_lang: Lang): void {
  // RED sentinel — see readStoredLang.
}

/** The language the current subtree renders in. */
export type LanguageValue = {
  lang: Lang
  setLang(lang: Lang): void
  t(key: CopyKey): string
  count(n: number): string
}

const LanguageContext = createContext<LanguageValue | null>(null)

/** Distributes the player route's language state to the screens below it. */
export function LanguageProvider(props: {
  lang: Lang
  setLang(lang: Lang): void
  children: ReactNode
}) {
  const value: LanguageValue = {
    lang: props.lang,
    setLang: props.setLang,
    t: (key) => translate(props.lang, key),
    count: (n) => playerCount(props.lang, n),
  }
  return createElement(LanguageContext.Provider, { value }, props.children)
}

/** The screens' accessor. Throws if used outside the provider (a build bug). */
export function useLanguage(): LanguageValue {
  const value = useContext(LanguageContext)
  if (value === null) {
    throw new Error('useLanguage must be used inside LanguageProvider')
  }
  return value
}
