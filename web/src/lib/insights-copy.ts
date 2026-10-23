import { useLanguage } from "./i18n";
export const INSIGHTS_COPY = {
  analytics: ["Auction analytics", "Phân tích đấu giá"],
  intro: [
    "Lifetime committed facts for published lots. Every payment value is simulated.",
    "Dữ liệu đã lưu suốt vòng đời của hiện vật đã công bố. Mọi giá trị thanh toán đều là mô phỏng.",
  ],
  event: ["Auction event", "Phiên đấu giá"],
  dataset: ["Lot dataset", "Nhóm hiện vật"],
  all: ["All", "Tất cả"],
  demo: ["Demo lots (lot or event)", "Hiện vật demo (hiện vật hoặc phiên)"],
  nonDemo: ["Non-demo lots", "Hiện vật không phải demo"],
  apply: ["Apply filters", "Áp dụng bộ lọc"],
  refresh: ["Refresh snapshot", "Làm mới dữ liệu"],
  loading: ["Loading…", "Đang tải…"],
  error: [
    "Data is unavailable or invalid. Please retry.",
    "Dữ liệu chưa khả dụng hoặc không hợp lệ. Vui lòng thử lại.",
  ],
  access: [
    "Operator capability required. Open your operator link, then retry.",
    "Cần quyền vận hành. Mở liên kết vận hành của bạn rồi thử lại.",
  ],
  retry: ["Retry", "Thử lại"],
  empty: [
    "No matching published lots.",
    "Không có hiện vật đã công bố phù hợp.",
  ],
  asOf: ["Snapshot as of", "Thời điểm dữ liệu"],
  coverage: ["Observed collection began", "Bắt đầu ghi nhận quan sát"],
  unknown: ["Not initialized", "Chưa khởi tạo"],
  compare: [
    "Compare events (scroll table horizontally if needed)",
    "So sánh phiên (cuộn ngang trong bảng nếu cần)",
  ],
  definitions: ["Metric definitions", "Định nghĩa chỉ số"],
  cohort: [
    "Lifetime published lots, attributed to their original event. Demo = lot demo OR event demo; actor flags stay separate.",
    "Hiện vật đã công bố suốt vòng đời, tính theo phiên ban đầu. Demo = hiện vật demo HOẶC phiên demo; cờ tài khoản được thống kê riêng.",
  ],
  formula: [
    "Bids count accepted rows. Bidders are distinct accounts with accepted bids; event bidder counts are not additive. Sell-through = sold / terminal (sold + unsold), unavailable when terminal is zero. The percentage is rounded for display; counts are exact. Pending includes every lot without a committed result.",
    "Lượt đặt giá là dòng đã chấp nhận. Người đặt giá là tài khoản riêng biệt có giá hợp lệ; không cộng số người giữa các phiên. Tỷ lệ bán = đã bán / đã kết thúc (đã bán + không bán), không xác định khi mẫu số bằng 0. Phần trăm được làm tròn khi hiển thị; số đếm là chính xác. Đang chờ gồm mọi hiện vật chưa có kết quả đã lưu.",
  ],
  moneyDefinition: [
    "Winning value includes unpaid wins. Simulated paid value counts each confirmed receipt once and equals paid winning value plus delivery fees.",
    "Giá thắng gồm cả hiện vật chưa thanh toán. Giá trị thanh toán mô phỏng tính mỗi biên nhận xác nhận một lần, bằng giá thắng đã thanh toán cộng phí giao nhận.",
  ],
  observations: [
    "Views are received page observations, not unique visitors or full traffic. Backfilled actions and current watchlist snapshots are separate; removed saves and historical views are unknown.",
    "Lượt xem là quan sát trang đã nhận, không phải số người hay toàn bộ lưu lượng. Dữ liệu bổ sung từ lịch sử và ảnh chụp danh sách đang lưu được tách riêng; không biết lượt lưu đã xóa hoặc lượt xem cũ.",
  ],
  interactions: [
    "Private aggregate interaction coverage",
    "Phạm vi tương tác tổng hợp riêng tư",
  ],
  lot_view: ["Page observations", "Quan sát trang"],
  watchlist_add: ["Watchlist additions", "Lượt thêm theo dõi"],
  bid_accepted: ["Accepted bids", "Giá được chấp nhận"],
  payment_confirmed: [
    "Confirmed simulated payments",
    "Thanh toán mô phỏng đã xác nhận",
  ],
  observed: ["Observed", "Đã quan sát"],
  backfilled: ["Backfilled history", "Bổ sung từ lịch sử"],
  snapshot: ["Current-membership snapshot", "Ảnh chụp thành viên đang lưu"],
  suggestions: ["Lots to explore", "Hiện vật nên khám phá"],
  rules: [
    "Rule-based suggestions · rules-v1 · not trained ML",
    "Gợi ý theo quy tắc · rules-v1 · không phải ML đã huấn luyện",
  ],
  publicFallback: [
    "Explore more from the collection.",
    "Khám phá thêm trong bộ sưu tập.",
  ],
  privateLabel: [
    "Objects selected around your interests.",
    "Hiện vật được chọn theo mối quan tâm của bạn.",
  ],
  accountUnavailable: [
    "Account validation is unavailable. Public suggestions remain available.",
    "Chưa xác nhận được tài khoản. Gợi ý công khai vẫn khả dụng.",
  ],
  privateError: [
    "Private suggestions could not be loaded. Public suggestions are shown separately.",
    "Chưa tải được gợi ý riêng. Gợi ý công khai hiển thị riêng.",
  ],
  noSuggestions: [
    "No eligible future-close lots to suggest.",
    "Không có hiện vật chưa kết thúc phù hợp để gợi ý.",
  ],
  category_and_era: ["Same category and era", "Cùng danh mục và thời kỳ"],
  category: ["Same category", "Cùng danh mục"],
  era: ["Same authored era", "Cùng thời kỳ được mô tả"],
  watchlist_category_and_era: [
    "Category and era from your watchlist",
    "Danh mục và thời kỳ trong danh sách theo dõi",
  ],
  watchlist_category: [
    "Category from your watchlist",
    "Danh mục trong danh sách theo dõi",
  ],
  watchlist_era: [
    "Authored era from your watchlist",
    "Thời kỳ mô tả trong danh sách theo dõi",
  ],
  closing_soon: [
    "Closing soon",
    "Sắp đóng",
  ],
  lots: ["Published lots", "Hiện vật đã công bố"],
  bids: ["Accepted bids", "Lượt đặt giá hợp lệ"],
  bidders: [
    "Distinct bidders (not additive)",
    "Người đặt giá riêng biệt (không cộng)",
  ],
  demo_bids: ["Demo actor bids", "Giá từ tài khoản demo"],
  non_demo_bids: ["Non-demo actor bids", "Giá từ tài khoản thường"],
  demo_bidders: [
    "Demo bidders (not additive)",
    "Người đặt giá demo (không cộng)",
  ],
  non_demo_bidders: [
    "Non-demo bidders (not additive)",
    "Người đặt giá thường (không cộng)",
  ],
  sold: ["Sold lots", "Hiện vật đã bán"],
  unsold: ["Unsold lots", "Hiện vật không bán"],
  terminal: ["Terminal results", "Kết quả đã kết thúc"],
  pending: ["Pending results", "Kết quả đang chờ"],
  sell_through: [
    "Sell-through (rounded percentage; exact operands)",
    "Tỷ lệ bán (phần trăm làm tròn; số đếm chính xác)",
  ],
  winning_minor: [
    "Winning value (includes unpaid)",
    "Giá thắng (gồm chưa thanh toán)",
  ],
  paid_orders: ["Simulated paid orders", "Đơn thanh toán mô phỏng"],
  unpaid_sold_orders: ["Unpaid sold orders", "Đơn thắng chưa thanh toán"],
  paid_winning_minor: [
    "Paid winning value (simulated)",
    "Giá thắng đã thanh toán (mô phỏng)",
  ],
  paid_fees_minor: [
    "Paid delivery fees (simulated)",
    "Phí giao nhận đã thanh toán (mô phỏng)",
  ],
  simulated_paid_minor: ["Simulated paid total", "Tổng thanh toán mô phỏng"],
} as const;
export type InsightsKey = keyof typeof INSIGHTS_COPY;
export function useInsightsCopy() {
  const { lang } = useLanguage();
  return {
    lang,
    i: (key: InsightsKey) => INSIGHTS_COPY[key][lang === "vi" ? 1 : 0],
  };
}
