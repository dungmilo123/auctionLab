import { describe, expect, it } from 'vitest'
import { COPY, DEFAULT_LANG, playerCount, readStoredLang, translate, type CopyKey } from './i18n'

// The approved copy table, transcribed by hand from 02-UI-SPEC.md §Copywriting
// Contract — NOT imported from the module — so a reworded string fails here
// instead of shipping to a classroom. Typing it as a full Record<CopyKey, …>
// means a new dictionary key is a compile error until its copy is approved here
// too, which is the same guarantee the typed dictionary gives on the other side.
const APPROVED: Record<CopyKey, { en: string; vi: string }> = {
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
}

const KEYS = Object.keys(APPROVED) as CopyKey[]

describe('the approved copy table', () => {
  it('serves the approved English copy for every key', () => {
    for (const key of KEYS) {
      expect(translate('en', key)).toBe(APPROVED[key].en)
    }
  })

  it('serves the approved Vietnamese copy for every key', () => {
    for (const key of KEYS) {
      expect(translate('vi', key)).toBe(APPROVED[key].vi)
    }
  })

  it('ships both languages for every key, and never an empty string', () => {
    for (const key of KEYS) {
      expect(COPY[key].en.length).toBeGreaterThan(0)
      expect(COPY[key].vi.length).toBeGreaterThan(0)
    }
  })
})

describe('the player count (lobby.count)', () => {
  it('uses the English plural rule', () => {
    expect(playerCount('en', 1)).toBe('1 player')
    expect(playerCount('en', 2)).toBe('2 players')
    expect(playerCount('en', 20)).toBe('20 players')
  })

  it('uses the single Vietnamese form at every count', () => {
    expect(playerCount('vi', 1)).toBe('1 người chơi')
    expect(playerCount('vi', 20)).toBe('20 người chơi')
  })
})

describe('the stored language', () => {
  it('defaults to English when storage is unavailable', () => {
    // The node test environment has no `window` at all, which is the same path
    // a phone in private mode takes: the read must degrade to the default
    // rather than throw.
    expect(readStoredLang()).toBe(DEFAULT_LANG)
    expect(DEFAULT_LANG).toBe('en')
  })
})
