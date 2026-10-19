import { useSiteCopy } from './site-copy'
import type { Lang } from './i18n'

export const CATALOG_COPY = {
  "management": {
    "en": "Catalog preparation",
    "vi": "Chuẩn bị danh mục"
  },
  "notice": {
    "en": "Save drafts and publish complete events. Bidding and purchases are unavailable.",
    "vi": "Lưu bản nháp và công bố phiên hoàn chỉnh. Đặt giá và mua hàng chưa khả dụng."
  },
  "lots": {
    "en": "Lots",
    "vi": "Hiện vật"
  },
  "events": {
    "en": "Auction events",
    "vi": "Phiên đấu giá"
  },
  "newLot": {
    "en": "New lot",
    "vi": "Tạo hiện vật"
  },
  "newEvent": {
    "en": "New event",
    "vi": "Tạo phiên"
  },
  "create": {
    "en": "Create draft",
    "vi": "Tạo bản nháp"
  },
  "save": {
    "en": "Save draft",
    "vi": "Lưu bản nháp"
  },
  "saving": {
    "en": "Saving…",
    "vi": "Đang lưu…"
  },
  "saved": {
    "en": "Saved to PostgreSQL.",
    "vi": "Đã lưu vào PostgreSQL."
  },
  "publishedSuccess": {
    "en": "Event and all member lots published.",
    "vi": "Đã công bố phiên và toàn bộ hiện vật."
  },
  "publishedNote": {
    "en": "Published records are read-only. Planned dates do not execute auctions.",
    "vi": "Bản đã công bố chỉ đọc. Lịch dự kiến không tự chạy đấu giá."
  },
  "draft": {
    "en": "Draft",
    "vi": "Bản nháp"
  },
  "published": {
    "en": "Published",
    "vi": "Đã công bố"
  },
  "assigned": {
    "en": "Assigned to event",
    "vi": "Đã gán vào phiên"
  },
  "unassigned": {
    "en": "Unassigned",
    "vi": "Chưa gán phiên"
  },
  "back": {
    "en": "Back to list",
    "vi": "Về danh sách"
  },
  "reload": {
    "en": "Reload committed state",
    "vi": "Tải lại dữ liệu đã lưu"
  },
  "loading": {
    "en": "Loading stored catalog…",
    "vi": "Đang tải danh mục đã lưu…"
  },
  "empty": {
    "en": "No records yet. Create a titled draft to begin.",
    "vi": "Chưa có dữ liệu. Tạo bản nháp có tiêu đề để bắt đầu."
  },
  "edit": {
    "en": "Open record",
    "vi": "Mở bản ghi"
  },
  "content": {
    "en": "Content",
    "vi": "Nội dung"
  },
  "identity": {
    "en": "Identity and category",
    "vi": "Định danh và danh mục"
  },
  "gallery": {
    "en": "Ordered gallery",
    "vi": "Thư viện ảnh có thứ tự"
  },
  "specs": {
    "en": "Ordered specifications",
    "vi": "Thông số có thứ tự"
  },
  "money": {
    "en": "Prices in USD",
    "vi": "Giá bằng USD"
  },
  "fulfillment": {
    "en": "Fulfillment",
    "vi": "Giao và nhận hàng"
  },
  "notes": {
    "en": "Private notes",
    "vi": "Ghi chú riêng"
  },
  "schedule": {
    "en": "Planned schedule",
    "vi": "Lịch dự kiến"
  },
  "membership": {
    "en": "Ordered event lots",
    "vi": "Hiện vật trong phiên theo thứ tự"
  },
  "available": {
    "en": "Available draft lots",
    "vi": "Hiện vật nháp có thể chọn"
  },
  "add": {
    "en": "Add",
    "vi": "Thêm"
  },
  "remove": {
    "en": "Remove",
    "vi": "Xóa khỏi danh sách"
  },
  "up": {
    "en": "Move up",
    "vi": "Chuyển lên"
  },
  "down": {
    "en": "Move down",
    "vi": "Chuyển xuống"
  },
  "saveMembership": {
    "en": "Save lot order",
    "vi": "Lưu thứ tự hiện vật"
  },
  "preview": {
    "en": "Preview saved public fields",
    "vi": "Xem trước trường công khai đã lưu"
  },
  "publish": {
    "en": "Publish saved event",
    "vi": "Công bố phiên đã lưu"
  },
  "previewNote": {
    "en": "Authorized preview of saved values. Unsaved fields are not included.",
    "vi": "Xem trước dữ liệu đã lưu, có kiểm tra quyền. Không bao gồm trường chưa lưu."
  },
  "timezone": {
    "en": "Times use your browser timezone",
    "vi": "Thời gian theo múi giờ trình duyệt"
  },
  "scheduleNote": {
    "en": "Save content and lot order to refresh server-derived closing times. Events may overlap.",
    "vi": "Lưu nội dung và thứ tự hiện vật để cập nhật giờ đóng do máy chủ tính. Các phiên có thể trùng lịch."
  },
  "mediaHelp": {
    "en": "Public local files only: /images/ or /site-assets/catalog/. No upload or remote URLs.",
    "vi": "Chỉ dùng tệp công khai cục bộ: /images/ hoặc /site-assets/catalog/. Không tải lên hay dùng URL ngoài."
  },
  "optionalHelp": {
    "en": "Incomplete drafts are allowed. Supplied values must be valid.",
    "vi": "Có thể lưu bản nháp chưa đầy đủ. Giá trị đã nhập phải hợp lệ."
  },
  "storyHelp": {
    "en": "Supplied story only; no independent authenticity claim.",
    "vi": "Chỉ là câu chuyện được cung cấp; không khẳng định xác thực độc lập."
  },
  "accessHelp": {
    "en": "Authorize at the Go admin gate on the same hostname, then return and reload. Use the entry link from the server console.",
    "vi": "Xác thực tại cổng quản trị Go trên cùng hostname, rồi trở lại và tải lại. Dùng liên kết từ console máy chủ."
  },
  "gate": {
    "en": "Open Go admin gate",
    "vi": "Mở cổng quản trị Go"
  },
  "recoveryHelp": {
    "en": "Failed saves retain your input. Reload replaces it with committed values; copy anything you wish to retain first.",
    "vi": "Lưu thất bại giữ nguyên nội dung đang nhập. Tải lại sẽ thay bằng dữ liệu đã lưu; hãy sao chép phần muốn giữ trước."
  },
  "pendingNote": {
    "en": "An operation is pending.",
    "vi": "Đang xử lý thao tác."
  },
  "unsaved": {
    "en": "Unsaved changes: save this section before preview or publication.",
    "vi": "Có thay đổi chưa lưu: lưu phần này trước khi xem trước hoặc công bố."
  },
  "title": {
    "en": "Title",
    "vi": "Tiêu đề"
  },
  "slug": {
    "en": "Slug",
    "vi": "Đường dẫn slug"
  },
  "category_id": {
    "en": "Category",
    "vi": "Danh mục"
  },
  "description": {
    "en": "Description",
    "vi": "Mô tả"
  },
  "brand": {
    "en": "Brand (if known)",
    "vi": "Thương hiệu (nếu biết)"
  },
  "era": {
    "en": "Era (if known)",
    "vi": "Thời kỳ (nếu biết)"
  },
  "story": {
    "en": "Supplied story / provenance",
    "vi": "Câu chuyện / nguồn gốc được cung cấp"
  },
  "condition": {
    "en": "Known condition and wear",
    "vi": "Tình trạng và dấu vết đã biết"
  },
  "path": {
    "en": "Local image path",
    "vi": "Đường dẫn ảnh cục bộ"
  },
  "alt": {
    "en": "Alternative text",
    "vi": "Văn bản thay thế"
  },
  "name": {
    "en": "Specification name",
    "vi": "Tên thông số"
  },
  "value": {
    "en": "Specification value",
    "vi": "Giá trị thông số"
  },
  "starting_price_minor": {
    "en": "Opening price (USD)",
    "vi": "Giá mở đầu (USD)"
  },
  "bid_increment_minor": {
    "en": "Bid increment (USD)",
    "vi": "Bước giá (USD)"
  },
  "shipping_fee_minor": {
    "en": "Delivery fee (USD)",
    "vi": "Phí giao hàng (USD)"
  },
  "shipping_enabled": {
    "en": "Enable delivery",
    "vi": "Cho phép giao hàng"
  },
  "pickup_enabled": {
    "en": "Enable pickup",
    "vi": "Cho phép nhận tại chỗ"
  },
  "shipping_instructions": {
    "en": "Delivery instructions",
    "vi": "Hướng dẫn giao hàng"
  },
  "pickup_instructions": {
    "en": "Pickup instructions",
    "vi": "Hướng dẫn nhận hàng"
  },
  "admin_notes": {
    "en": "Admin notes (private)",
    "vi": "Ghi chú quản trị (riêng tư)"
  },
  "theme": {
    "en": "Theme",
    "vi": "Chủ đề"
  },
  "cover_path": {
    "en": "Local cover path",
    "vi": "Đường dẫn ảnh bìa cục bộ"
  },
  "cover_alt": {
    "en": "Cover alternative text",
    "vi": "Văn bản thay thế ảnh bìa"
  },
  "opens_at": {
    "en": "Common opening",
    "vi": "Giờ mở chung"
  },
  "first_closes_at": {
    "en": "First lot close",
    "vi": "Giờ đóng hiện vật đầu"
  },
  "close_interval_ms": {
    "en": "Closing interval (milliseconds)",
    "vi": "Khoảng cách đóng (mili giây)"
  },
  "planned_end": {
    "en": "Planned end",
    "vi": "Giờ kết thúc dự kiến"
  },
  "lot_ids": {
    "en": "Event lots",
    "vi": "Hiện vật trong phiên"
  },
  "images": {
    "en": "Images",
    "vi": "Hình ảnh"
  },
  "specifications": {
    "en": "Specifications",
    "vi": "Thông số"
  },
  "currency": {
    "en": "Currency",
    "vi": "Tiền tệ"
  },
  "access_required": {
    "en": "Admin access is required.",
    "vi": "Cần quyền quản trị."
  },
  "unavailable": {
    "en": "The application server or database is unavailable.",
    "vi": "Máy chủ ứng dụng hoặc cơ sở dữ liệu chưa khả dụng."
  },
  "not_found": {
    "en": "This record or selection was not found.",
    "vi": "Không tìm thấy bản ghi hoặc lựa chọn này."
  },
  "validation": {
    "en": "Check the fields below. Your input has been retained.",
    "vi": "Kiểm tra các trường bên dưới. Nội dung đang nhập được giữ nguyên."
  },
  "stale_version": {
    "en": "A newer version exists. Reload before saving again.",
    "vi": "Đã có phiên bản mới hơn. Tải lại trước khi lưu tiếp."
  },
  "duplicate_slug": {
    "en": "That slug is already in use.",
    "vi": "Đường dẫn slug đã được dùng."
  },
  "already_assigned": {
    "en": "A selected lot belongs to another event. Detach it there first.",
    "vi": "Hiện vật đã thuộc phiên khác. Hãy gỡ khỏi phiên đó trước."
  },
  "duplicate_lot": {
    "en": "Each lot can appear only once.",
    "vi": "Mỗi hiện vật chỉ được xuất hiện một lần."
  },
  "missing_lot": {
    "en": "A selected lot no longer exists. Reload the list.",
    "vi": "Hiện vật được chọn không còn tồn tại. Tải lại danh sách."
  },
  "transaction_conflict": {
    "en": "The transaction conflicted. Reload and review before retrying.",
    "vi": "Giao dịch bị xung đột. Tải lại và kiểm tra trước khi thử tiếp."
  },
  "commit_unknown": {
    "en": "The save outcome is uncertain. Reload committed state before any retry.",
    "vi": "Chưa xác định được kết quả lưu. Tải lại dữ liệu đã lưu trước khi thử tiếp."
  },
  "storage_error": {
    "en": "The save failed. Your input is retained; reload to inspect committed state.",
    "vi": "Lưu thất bại. Nội dung được giữ nguyên; tải lại để kiểm tra dữ liệu đã lưu."
  },
  "request_rejected": {
    "en": "The request was rejected. Reload from the application origin.",
    "vi": "Yêu cầu bị từ chối. Tải lại từ địa chỉ ứng dụng."
  },
  "json_required": {
    "en": "The server requires a JSON request. Reload the editor.",
    "vi": "Máy chủ yêu cầu JSON. Tải lại trình chỉnh sửa."
  },
  "malformed_json": {
    "en": "The request could not be read. Reload the editor.",
    "vi": "Không đọc được yêu cầu. Tải lại trình chỉnh sửa."
  },
  "required": {
    "en": "Required before publication.",
    "vi": "Cần có trước khi công bố."
  },
  "invalid": {
    "en": "Enter a valid value.",
    "vi": "Nhập giá trị hợp lệ."
  },
  "moneyError": {
    "en": "Use a nonnegative USD decimal with at most two decimal places, up to 10000000000.00. Increments must be positive.",
    "vi": "Nhập số USD không âm, tối đa hai chữ số thập phân và không quá 10000000000.00. Bước giá phải dương."
  },
  "usd_only": {
    "en": "Only USD is supported.",
    "vi": "Chỉ hỗ trợ USD."
  },
  "scheduleError": {
    "en": "Use valid times and a positive interval; closing must follow opening.",
    "vi": "Nhập thời gian hợp lệ và khoảng cách dương; giờ đóng phải sau giờ mở."
  },
  "schedule_overflow": {
    "en": "The derived closing schedule exceeds the supported range.",
    "vi": "Lịch đóng tính ra vượt phạm vi hỗ trợ."
  },
  "future": {
    "en": "Opening must be in the future when publishing.",
    "vi": "Giờ mở phải ở tương lai khi công bố."
  },
  "media_path": {
    "en": "Use a canonical supported local image path.",
    "vi": "Dùng đường dẫn ảnh cục bộ hợp lệ."
  },
  "media_missing": {
    "en": "Install a readable local image before publication.",
    "vi": "Cần cài ảnh cục bộ đọc được trước khi công bố."
  },
  "reloadRequired": {
    "en": "Reload is required before another mutation.",
    "vi": "Cần tải lại trước thao tác thay đổi tiếp theo."
  }
} as const
export type CatalogKey = keyof typeof CATALOG_COPY
export function catalogText(lang: Lang, key: string): string { const normalized = key === 'published' ? 'publishedNote' : key === 'money' ? 'moneyError' : key === 'schedule' ? 'scheduleError' : key; return CATALOG_COPY[normalized as CatalogKey]?.[lang] ?? CATALOG_COPY.invalid[lang] }
export function useCatalogCopy() { const { lang } = useSiteCopy(); return { lang, c: (key: CatalogKey) => CATALOG_COPY[key][lang], errorText: (code: string) => catalogText(lang, code), fieldText: (field: string) => CATALOG_COPY[field.split('.').filter(part => !/^\d+$/.test(part)).at(-1) as CatalogKey]?.[lang] ?? field } }
