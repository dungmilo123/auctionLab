import { useLanguage } from "./i18n";
export const ORDER_COPY = {
  wins: { en: "Unpaid wins", vi: "Hiện vật thắng chưa thanh toán" },
  checkout: { en: "Winner checkout", vi: "Thanh toán hiện vật thắng" },
  simulation: {
    en: "Payment simulation only. No money is charged. Do not enter card or bank details.",
    vi: "Chỉ thanh toán. Không thu tiền. Không nhập thông tin thẻ hoặc ngân hàng.",
  },
  loading: { en: "Loading your purchase…", vi: "Đang tải giao dịch của bạn…" },
  empty: {
    en: "You have no unpaid wins.",
    vi: "Bạn không có hiện vật thắng chưa thanh toán.",
  },
  unavailable: {
    en: "Purchase service unavailable. Your payment outcome is not established. Read the stored order to recover.",
    vi: "Dịch vụ giao dịch chưa khả dụng. Chưa xác định kết quả thanh toán. Tải lại đơn đã lưu để kiểm tra.",
  },
  missing: {
    en: "Order not found for this account.",
    vi: "Không tìm thấy đơn hàng của tài khoản này.",
  },
  retry: { en: "Read stored order", vi: "Tải đơn đã lưu" },
  open: { en: "Continue to checkout", vi: "Tiếp tục thanh toán" },
  contactDetails: { en: "Contact details", vi: "Thông tin liên hệ" },
  name: { en: "Contact name", vi: "Tên liên hệ" },
  email: { en: "Contact email", vi: "Email liên hệ" },
  phone: { en: "Contact phone", vi: "Số điện thoại liên hệ" },
  fulfillment: { en: "Fulfillment", vi: "Hình thức nhận hàng" },
  shipping: { en: "Shipping", vi: "Giao hàng" },
  pickup: { en: "Pickup", vi: "Nhận trực tiếp" },
  line1: { en: "Address line 1", vi: "Địa chỉ dòng 1" },
  line2: {
    en: "Address line 2 (optional)",
    vi: "Địa chỉ dòng 2 (không bắt buộc)",
  },
  locality: { en: "City / locality", vi: "Thành phố / địa phương" },
  region: { en: "Region (optional)", vi: "Tỉnh / vùng (không bắt buộc)" },
  postal_code: {
    en: "Postal code (optional)",
    vi: "Mã bưu chính (không bắt buộc)",
  },
  country: { en: "Country (two-letter code)", vi: "Quốc gia (mã hai chữ cái)" },
  payment: { en: "Payment method", vi: "Phương thức thanh toán" },
  "demo-card": { en: "Card", vi: "Thẻ" },
  "demo-bank-transfer": {
    en: "Bank transfer",
    vi: "Chuyển khoản",
  },
  review: { en: "Review order", vi: "Kiểm tra đơn hàng" },
  reviewTitle: {
    en: "Review order",
    vi: "Kiểm tra đơn hàng",
  },
  edit: { en: "Edit details", vi: "Sửa thông tin" },
  confirm: {
    en: "Confirm payment",
    vi: "Xác nhận thanh toán",
  },
  pending: {
    en: "Confirming payment. Please wait…",
    vi: "Đang xác nhận thanh toán. Vui lòng chờ…",
  },
  unknown: {
    en: "Payment outcome unknown. Recover the stored order before continuing.",
    vi: "Chưa xác định kết quả thanh toán. Kiểm tra đơn đã lưu trước khi tiếp tục.",
  },
  paid: {
    en: "Payment confirmed",
    vi: "Đã xác nhận thanh toán",
  },
  receipt: { en: "Receipt", vi: "Biên nhận" },
  order: { en: "Order", vi: "Đơn hàng" },
  paidAt: { en: "Confirmed at", vi: "Thời gian xác nhận" },
  item: { en: "Winning amount", vi: "Giá thắng" },
  fee: { en: "Fulfillment fee", vi: "Phí nhận hàng" },
  total: { en: "Total", vi: "Tổng cộng" },
  instructions: { en: "Published instructions", vi: "Hướng dẫn đã công bố" },
  condition: { en: "Condition", vi: "Tình trạng" },
  validation: {
    en: "Check the highlighted fields. Nothing has been paid.",
    vi: "Kiểm tra các trường được đánh dấu. Chưa thanh toán.",
  },
  required: { en: "Required", vi: "Bắt buộc" },
  invalid: {
    en: "Invalid value or length",
    vi: "Giá trị hoặc độ dài không hợp lệ",
  },
  conflict: {
    en: "This request ID was already accepted with different details. Read the stored order.",
    vi: "Mã yêu cầu này đã được chấp nhận với thông tin khác. Tải đơn đã lưu.",
  },
  history: {
    en: "View your orders and delivery timeline.",
    vi: "Xem đơn hàng và tiến độ nhận hàng.",
  },
  demo: { en: "Demo record", vi: "Dữ liệu minh họa" },
  viewPaid: { en: "View payment confirmation", vi: "Xem xác nhận thanh toán" },
} as const;
export type OrderKey = keyof typeof ORDER_COPY;
export function useOrderCopy() {
  const { lang } = useLanguage();
  return { lang, o: (key: OrderKey) => ORDER_COPY[key][lang] };
}
