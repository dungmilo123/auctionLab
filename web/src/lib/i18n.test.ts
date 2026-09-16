import { describe, expect, it } from 'vitest'
import {
  COPY,
  DEFAULT_LANG,
  LIVE_PARAM_KEYS,
  fill,
  playerCount,
  readStoredLang,
  translate,
  type CopyKey,
} from './i18n'

// The approved copy table, transcribed by hand from 03-UI-SPEC.md §Copywriting
// Contract (Phase 2's rows carried verbatim, Phase 3's 105 appended, Phase 4's
// one QR accessible name added by 04-06) — NOT imported from the module — so a
// reworded string fails here instead of shipping to a classroom. Typing it as a
// full Record<CopyKey, …> means a new dictionary key is a compile error until
// its copy is approved here too, which is the same guarantee the typed
// dictionary gives on the other side.
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

  // Phase 3 — shared vocabulary
  'amount.caption': { en: 'VND', vi: 'VND' },
  'auction.english': { en: 'English auction', vi: 'Đấu giá kiểu Anh' },
  'auction.dutch': { en: 'Dutch auction', vi: 'Đấu giá kiểu Hà Lan' },
  'auction.sealedFirst': { en: 'First-price sealed bid', vi: 'Đấu giá kín — trả giá cao nhất' },
  'auction.vickrey': { en: 'Vickrey (second-price)', vi: 'Vickrey — trả giá cao nhì' },
  'phase.LOBBY': { en: 'Lobby', vi: 'Phòng chờ' },
  'phase.ITEM_INTRO': { en: 'Item intro', vi: 'Giới thiệu item' },
  'phase.LIVE': { en: 'Live', vi: 'Đang đấu' },
  'phase.SETTLED': { en: 'Settled', vi: 'Đã chốt' },
  'phase.FINALE': { en: 'Finale', vi: 'Chung kết' },
  'item.status.pending': { en: 'Pending', vi: 'Đang chờ' },
  'item.status.intro': { en: 'Intro', vi: 'Đang giới thiệu' },
  'item.status.live': { en: 'Live', vi: 'Đang đấu' },
  'item.status.settled': { en: 'Settled', vi: 'Đã chốt' },
  'item.status.skipped': { en: 'Skipped', vi: 'Đã bỏ qua' },

  // Phase 3 — ITEM_INTRO
  'intro.typeLabel': { en: 'Auction type', vi: 'Loại đấu giá' },
  'intro.hintLabel': { en: 'Hint range', vi: 'Khoảng gợi ý' },
  'intro.hintNone': { en: 'No hint for this item', vi: 'Item này không có gợi ý' },
  'intro.startLabel': { en: 'Start price', vi: 'Giá khởi điểm' },
  'intro.waiting': { en: 'Waiting for the host to start…', vi: 'Chờ người dẫn bắt đầu…' },

  // Phase 3 — LIVE (English)
  'live.priceLabel': { en: 'Current price', vi: 'Giá hiện tại' },
  'live.timeLabel': { en: 'Time left', vi: 'Thời gian còn lại' },
  'live.topBidderLabel': { en: 'Top bidder', vi: 'Đang dẫn giá' },
  'live.noBidder': { en: 'No bids yet', vi: 'Chưa có ai đặt giá' },
  'live.nextBidLabel': { en: 'Next valid bid', vi: 'Mức đặt tối thiểu' },
  'live.bidLabel': { en: 'Your bid', vi: 'Giá bạn đặt' },
  'live.placeBid': { en: 'Place bid', vi: 'Đặt giá' },
  'live.bidding': { en: 'Placing…', vi: 'Đang đặt…' },
  'live.quick10k': { en: '+10k', vi: '+10k' },
  'live.quick50k': { en: '+50k', vi: '+50k' },
  'live.quick100k': { en: '+100k', vi: '+100k' },
  'live.outbid': { en: "You've been outbid!", vi: 'Bạn đã bị vượt giá!' },
  'live.rebid': { en: 'Rebid at {amount}', vi: 'Đặt lại {amount}' },
  'live.budgetLow': {
    en: 'Out of budget for this item — your budget is {budget} VND, the minimum bid is {min} VND. You can keep watching.',
    vi: 'Hết ngân sách cho item này — ngân sách {budget} VND, mức đặt tối thiểu {min} VND. Bạn vẫn xem tiếp được.',
  },
  'live.typePending': {
    en: "This auction type isn't available yet.",
    vi: 'Loại đấu giá này chưa chạy được.',
  },

  // Phase 3 — bid rejection reasons
  'reason.invalidAmount': { en: 'Enter a whole number of VND.', vi: 'Nhập một số nguyên VND.' },
  'reason.windowClosed': {
    en: 'Bidding is closed for this item.',
    vi: 'Đã đóng nhận đặt giá cho item này.',
  },
  'reason.belowMinimum': {
    en: 'Below the minimum — bid at least {min} VND.',
    vi: 'Thấp hơn mức tối thiểu — đặt ít nhất {min} VND.',
  },
  'reason.exceedsBudget': {
    en: "That's more than your budget ({budget} VND).",
    vi: 'Vượt quá ngân sách của bạn ({budget} VND).',
  },
  'reason.alreadyLeading': { en: "You're already the top bidder.", vi: 'Bạn đang là người dẫn giá.' },

  // Phase 3 — SETTLED
  'settle.soldHeading': { en: 'Sold!', vi: 'Đã bán!' },
  'settle.unsoldHeading': { en: 'Unsold', vi: 'Không bán được' },
  'settle.winnerLabel': { en: 'Winner', vi: 'Người thắng' },
  'settle.priceLabel': { en: 'Winning price', vi: 'Giá thắng' },
  'settle.truthLabel': { en: 'True value', vi: 'Giá trị thật' },
  'settle.deltaLabel': { en: 'Budget change', vi: 'Thay đổi ngân sách' },
  'settle.youWon': { en: 'You won this item!', vi: 'Bạn thắng item này!' },
  'settle.unsoldBody': {
    en: 'No one bid, so no budgets changed.',
    vi: 'Không ai đặt giá nên không ai bị trừ ngân sách.',
  },
  'settle.waiting': { en: 'Waiting for the next item…', vi: 'Chờ item tiếp theo…' },

  // Phase 3 — FINALE placeholder
  'finale.heading': { en: 'Finale', vi: 'Chung kết' },
  'finale.body': {
    en: "That's the end of the auction. Thanks for playing!",
    vi: 'Buổi đấu giá đã kết thúc. Cảm ơn bạn đã chơi!',
  },

  // Phase 3 — admin
  'admin.h1': { en: 'Admin', vi: 'Quản trị' },
  'admin.noKeyNote': {
    en: 'No admin key yet — Phase 4 adds it. Anyone on this network can open this page.',
    vi: 'Chưa có khoá quản trị — Phase 4 sẽ thêm. Ai trong mạng cũng mở được trang này.',
  },
  'admin.controlHeading': { en: 'Control', vi: 'Điều khiển' },
  'admin.phaseLabel': { en: 'Phase', vi: 'Giai đoạn' },
  'admin.currentItemLabel': { en: 'Current item', vi: 'Item hiện tại' },
  'admin.noCurrentItem': { en: 'None', vi: 'Chưa có' },
  'admin.start': { en: 'Start item', vi: 'Bắt đầu item' },
  'admin.forceEnd': { en: 'Force end', vi: 'Kết thúc ngay' },
  'admin.next': { en: 'Next item', vi: 'Item tiếp theo' },
  'admin.skipItem': { en: 'Skip item', vi: 'Bỏ qua item' },
  'admin.noPendingItem': { en: 'No pending items left.', vi: 'Không còn item nào đang chờ.' },
  'admin.forceEndConfirm': {
    en: "End this item now? It settles at the current price — this can't be undone.",
    vi: 'Kết thúc item ngay? Item sẽ chốt ở mức giá hiện tại — không thể hoàn tác.',
  },
  'admin.dontEnd': { en: "Don't end", vi: 'Không kết thúc' },
  'admin.itemsHeading': { en: 'Items', vi: 'Danh sách item' },
  'admin.newItem': { en: 'Add item', vi: 'Thêm item' },
  'admin.emptyList': { en: 'No items yet — add the first one.', vi: 'Chưa có item nào — thêm item đầu tiên.' },
  'admin.moveUp': { en: 'Up', vi: 'Lên' },
  'admin.moveDown': { en: 'Down', vi: 'Xuống' },
  'admin.moveUpLabel': { en: 'Move {name} up', vi: 'Di chuyển {name} lên' },
  'admin.moveDownLabel': { en: 'Move {name} down', vi: 'Di chuyển {name} xuống' },
  'admin.edit': { en: 'Edit', vi: 'Sửa' },
  'admin.editLabel': { en: 'Edit {name}', vi: 'Sửa {name}' },
  'admin.delete': { en: 'Delete', vi: 'Xoá' },
  'admin.deleteLabel': { en: 'Delete {name}', vi: 'Xoá {name}' },
  'admin.deleteConfirm': {
    en: 'Delete “{name}”? This can\'t be undone.',
    vi: 'Xoá “{name}”? Không thể hoàn tác.',
  },
  'admin.keepItem': { en: 'Keep item', vi: 'Giữ item' },
  'admin.lockedNote': {
    en: "Items that are live or settled can't be edited or deleted.",
    vi: 'Item đang đấu hoặc đã chốt thì không sửa hay xoá được.',
  },
  'admin.editorNew': { en: 'New item', vi: 'Item mới' },
  'admin.editorEdit': { en: 'Edit item', vi: 'Sửa item' },
  'admin.field.name': { en: 'Name', vi: 'Tên item' },
  'admin.field.auctionType': { en: 'Auction type', vi: 'Loại đấu giá' },
  'admin.field.truthValue': { en: 'True value', vi: 'Giá trị thật' },
  'admin.field.hintMin': { en: 'Hint min', vi: 'Gợi ý thấp nhất' },
  'admin.field.hintMax': { en: 'Hint max', vi: 'Gợi ý cao nhất' },
  'admin.field.startPrice': { en: 'Start price', vi: 'Giá khởi điểm' },
  'admin.field.minIncrement': { en: 'Minimum increment', vi: 'Bước giá tối thiểu' },
  'admin.field.durationMs': { en: 'Duration (ms)', vi: 'Thời lượng (ms)' },
  'admin.field.antiSnipe': { en: 'Anti-snipe', vi: 'Chống đặt giá phút chót' },
  'admin.antiSnipeOff': { en: 'Off', vi: 'Tắt' },
  'admin.antiSnipeExtend': { en: 'Extend', vi: 'Gia hạn' },
  'admin.field.antiSnipeWindowMs': { en: 'Extension window (ms)', vi: 'Khung gia hạn (ms)' },
  'admin.field.maxExtensions': {
    en: 'Max extensions (blank = no limit)',
    vi: 'Số lần gia hạn tối đa (trống = không giới hạn)',
  },
  'admin.field.floorPrice': { en: 'Floor price', vi: 'Giá sàn' },
  'admin.field.decrementStep': { en: 'Decrement step', vi: 'Bước giảm' },
  'admin.field.decrementIntervalMs': { en: 'Decrement interval (ms)', vi: 'Chu kỳ giảm (ms)' },
  'admin.discardEdits': { en: 'Discard edits', vi: 'Bỏ thay đổi' },
  'admin.save': { en: 'Save item', vi: 'Lưu item' },
  'admin.saving': { en: 'Saving…', vi: 'Đang lưu…' },
  'admin.actionRejected': {
    en: 'The server refused that — check the values and try again.',
    vi: 'Máy chủ từ chối — kiểm tra lại giá trị rồi thử lại.',
  },
  'admin.notAuthorized': {
    en: "This connection isn't authorized — reopen the admin link from the server log.",
    vi: 'Kết nối này không có quyền — mở lại link quản trị trong log máy chủ.',
  },
  'admin.err.name': { en: 'Enter 1 to 80 characters.', vi: 'Nhập từ 1 đến 80 ký tự.' },
  'admin.err.notNegative': { en: 'Enter a whole number of 0 or more.', vi: 'Nhập số nguyên từ 0 trở lên.' },
  'admin.err.positive': { en: 'Enter a whole number of 1 or more.', vi: 'Nhập số nguyên từ 1 trở lên.' },
  'admin.err.hintOrder': {
    en: "Hint min can't be above hint max.",
    vi: 'Gợi ý thấp nhất không được lớn hơn gợi ý cao nhất.',
  },

  // Phase 4 — the shared QR component's accessible name (04-06)
  'screen.qrTitle': { en: 'Join QR code', vi: 'Mã QR vào phòng' },
}

const KEYS = Object.keys(APPROVED) as CopyKey[]

// The documented values for each placeholder, transcribed from the same UI-SPEC
// table (a VND amount for {amount}/{min}/{budget}, an item name for {name}).
const DOCUMENTED_VALUES: Record<keyof typeof LIVE_PARAM_KEYS, Record<string, string>> = {
  'live.rebid': { amount: '1.250.000' },
  'live.budgetLow': { budget: '500.000', min: '600.000' },
  'reason.belowMinimum': { min: '600.000' },
  'reason.exceedsBudget': { budget: '500.000' },
  'admin.moveUpLabel': { name: 'Đèn bàn' },
  'admin.moveDownLabel': { name: 'Đèn bàn' },
  'admin.editLabel': { name: 'Đèn bàn' },
  'admin.deleteLabel': { name: 'Đèn bàn' },
  'admin.deleteConfirm': { name: 'Đèn bàn' },
}

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

  it('carries exactly the 171 approved keys (25 carried + 105 Phase 3 + 42 Phase 4 − 1 deleted)', () => {
    expect(KEYS).toHaveLength(171)
  })
})

describe('placeholder substitution (fill)', () => {
  it('replaces every token it was given', () => {
    expect(fill('Rebid at {amount}', { amount: '1.250.000' })).toBe('Rebid at 1.250.000')
    expect(fill('{a} and {b}', { a: 'x', b: 'y' })).toBe('x and y')
  })

  it('leaves an unmatched token verbatim rather than blanking it', () => {
    expect(fill('Rebid at {amount}', {})).toBe('Rebid at {amount}')
    expect(fill('{budget} / {min}', { budget: '500.000' })).toBe('500.000 / {min}')
    expect(fill('no tokens here', { amount: 'ignored' })).toBe('no tokens here')
  })

  it('fills each closed-list template with its documented values in both languages', () => {
    for (const [key, params] of Object.entries(DOCUMENTED_VALUES) as [
      keyof typeof LIVE_PARAM_KEYS,
      Record<string, string>,
    ][]) {
      const names = LIVE_PARAM_KEYS[key]
      // The documented values and the declared names must describe the same
      // template: a parameter the list forgot would leave a brace behind.
      expect([...names].sort()).toEqual(Object.keys(params).sort())
      for (const lang of ['en', 'vi'] as const) {
        const filled = fill(translate(lang, key), params)
        expect(filled).not.toContain('{')
        expect(filled).not.toContain('}')
      }
    }
  })

  it('declares exactly the keys whose copy contains a brace', () => {
    for (const key of KEYS) {
      const templated = COPY[key].en.includes('{') || COPY[key].vi.includes('{')
      expect(key in LIVE_PARAM_KEYS).toBe(templated)
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
