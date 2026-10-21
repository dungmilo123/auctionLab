import { useLanguage } from "./i18n";
export const FULFILLMENT_COPY = {
  history: { en: "Order history", vi: "Lịch sử đơn hàng" },
  detail: { en: "Order details", vi: "Chi tiết đơn hàng" },
  simulation: {
    en: "Simulated payment and fulfillment. Shipment information is entered manually; no delivery service is contacted.",
    vi: "Thanh toán và nhận hàng mô phỏng. Thông tin vận chuyển được nhập thủ công; không kết nối đơn vị giao hàng.",
  },
  loading: { en: "Loading stored orders…", vi: "Đang tải đơn hàng đã lưu…" },
  empty: { en: "No orders match.", vi: "Không có đơn hàng phù hợp." },
  unavailable: {
    en: "Orders could not be loaded. Read the stored records to recover.",
    vi: "Không tải được đơn hàng. Tải lại dữ liệu đã lưu để kiểm tra.",
  },
  missing: { en: "Order not found.", vi: "Không tìm thấy đơn hàng." },
  access: {
    en: "Operator access required. Open the application server’s admin gate, then return here.",
    vi: "Cần quyền vận hành. Mở cổng quản trị trên máy chủ ứng dụng, rồi quay lại đây.",
  },
  gate: { en: "Open admin gate", vi: "Mở cổng quản trị" },
  retry: { en: "Reload stored order", vi: "Tải lại đơn đã lưu" },
  reload: { en: "Reload orders", vi: "Tải lại đơn hàng" },
  awaiting_payment: { en: "Awaiting payment", vi: "Chờ thanh toán" },
  paid: { en: "Paid (simulated)", vi: "Đã thanh toán mô phỏng" },
  preparing: { en: "Preparing", vi: "Đang chuẩn bị" },
  shipped: { en: "Shipped (simulated)", vi: "Đang giao mô phỏng" },
  ready_for_pickup: { en: "Ready for pickup", vi: "Sẵn sàng nhận" },
  completed: { en: "Completed (simulated)", vi: "Hoàn tất mô phỏng" },
  timeline: { en: "Status timeline", vi: "Lịch sử trạng thái" },
  created: { en: "Order created", vi: "Đã tạo đơn" },
  result: { en: "Auction result recorded", vi: "Đã ghi nhận kết quả đấu giá" },
  closed: { en: "Auction closed", vi: "Đấu giá kết thúc" },
  system: { en: "System", vi: "Hệ thống" },
  payment: { en: "Owner payment", vi: "Người mua thanh toán" },
  operator: { en: "Operator", vi: "Người vận hành" },
  status: { en: "Status", vi: "Trạng thái" },
  method: { en: "Selected method", vi: "Hình thức đã chọn" },
  all: { en: "All", vi: "Tất cả" },
  shipping: { en: "Shipping", vi: "Giao hàng" },
  pickup: { en: "Pickup", vi: "Nhận trực tiếp" },
  unselected: { en: "Unselected", vi: "Chưa chọn" },
  filter: { en: "Apply filters", vi: "Lọc đơn hàng" },
  invalidQuery: {
    en: "Invalid filters or order selection. Reset the URL to recover.",
    vi: "Bộ lọc hoặc đơn đã chọn không hợp lệ. Đặt lại URL để tiếp tục.",
  },
  resetFilters: { en: "Reset filters", vi: "Đặt lại bộ lọc" },
  carrier: {
    en: "Carrier (maximum 120 characters)",
    vi: "Đơn vị vận chuyển (tối đa 120 ký tự)",
  },
  tracking: {
    en: "Tracking reference (maximum 200 characters)",
    vi: "Mã vận chuyển (tối đa 200 ký tự)",
  },
  startPreparing: { en: "Start preparation", vi: "Bắt đầu chuẩn bị" },
  markShipped: {
    en: "Confirm simulated shipment",
    vi: "Xác nhận giao hàng mô phỏng",
  },
  markReady: { en: "Mark ready for pickup", vi: "Xác nhận sẵn sàng nhận" },
  markCompleted: {
    en: "Confirm simulated completion",
    vi: "Xác nhận hoàn tất mô phỏng",
  },
  review: { en: "Review shipment", vi: "Kiểm tra thông tin giao hàng" },
  reviewHint: {
    en: "Review these values before shipment. They cannot be edited after confirmation.",
    vi: "Kiểm tra thông tin trước khi giao. Không thể sửa sau khi xác nhận.",
  },
  edit: { en: "Edit shipment", vi: "Sửa thông tin giao hàng" },
  pending: { en: "Saving progress…", vi: "Đang lưu tiến độ…" },
  conflict: {
    en: "This order changed. Reload its stored state before another deliberate action.",
    vi: "Đơn hàng đã thay đổi. Tải lại trạng thái đã lưu trước khi thao tác tiếp.",
  },
  unknown: {
    en: "Update outcome unknown. Reload must succeed before any further action.",
    vi: "Chưa xác định kết quả cập nhật. Phải tải lại thành công trước khi thao tác tiếp.",
  },
  saved: {
    en: "Stored progress confirmed.",
    vi: "Đã xác nhận tiến độ được lưu.",
  },
  invalid: {
    en: "Check required text, character limits and control characters.",
    vi: "Kiểm tra nội dung bắt buộc, giới hạn ký tự và ký tự điều khiển.",
  },
  select: { en: "Inspect order", vi: "Xem đơn hàng" },
  receipt: { en: "View payment confirmation", vi: "Xem xác nhận thanh toán" },
  checkout: { en: "Continue to checkout", vi: "Tiếp tục thanh toán" },
  demo: { en: "Demo record", vi: "Dữ liệu minh họa" },
  winning: { en: "Winning amount", vi: "Giá thắng" },
  total: { en: "Paid total", vi: "Tổng đã thanh toán" },
  contact: { en: "Submitted contact", vi: "Liên hệ đã gửi" },
  instructions: { en: "Published instructions", vi: "Hướng dẫn đã công bố" },
  order: { en: "Order", vi: "Đơn hàng" },
  receiptID: { en: "Simulated receipt", vi: "Biên nhận mô phỏng" },
  fee: { en: "Fulfillment fee", vi: "Phí nhận hàng" },
} as const;
export type FulfillmentKey = keyof typeof FULFILLMENT_COPY;
export function useFulfillmentCopy() {
  const { lang } = useLanguage();
  return { lang, f: (key: FulfillmentKey) => FULFILLMENT_COPY[key][lang] };
}
