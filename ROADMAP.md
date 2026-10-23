# AuctionLab — Vintage Auction Website Roadmap

Ngày lập: 2026-10-19.

Trạng thái: Changes 1–3 đã được archive. Change 4 `add-bidder-accounts-and-watchlists` đã triển khai và được archive ngày 2026-10-20 với 32/34 task; hai kiểm tra zoom trình duyệt 200% (7.3, 8.3) vẫn chờ xác minh thủ công, chưa nghiệm thu đầy đủ. Change 5 `enable-timed-auction-bidding` đã triển khai và được archive ngày 2026-10-20 với 38/39 task; task 8.4 (zoom trình duyệt 200% và reduced motion thực tế) vẫn chưa xác minh. Change 6 `add-winner-checkout-and-demo-payment` đã triển khai checkout riêng cho người thắng và thanh toán mô phỏng ngày 2026-10-21; kiểm tra PostgreSQL, test/race/build và diễn tập Vite/embedded đã đạt. Đã archive ngày 2026-10-21 với 35/36 task; nghiệm thu accessibility 7.3 còn thiếu zoom trình duyệt 200%, reduced motion thực tế và toàn bộ ma trận long-title/field-error, chưa nghiệm thu đầy đủ. Change 7 `add-order-history-and-demo-fulfillment` đã triển khai và archive ngày 2026-10-21: lịch sử đơn, tiến độ nhận hàng mô phỏng và prepare/status/reset cho schema demo riêng; nghiệm thu là 39/41 task, còn 7.5–7.6 (zoom trình duyệt 200% và reduced motion hoạt động cùng toàn bộ ma trận) chưa xác minh. Change 8 `add-auction-analytics-and-recommendations` đã triển khai và xác minh ngày 2026-10-22: analytics tổng hợp riêng, gợi ý theo quy tắc, tương tác tối thiểu và migration 6; ML extension vẫn là định hướng. Các giới hạn nghiệm thu Change 4–7 giữ nguyên.

## 1. Mục tiêu và phạm vi

Xây một website đấu giá đồ vintage mới, có hành trình demo hoàn chỉnh:

```text
Khám phá phiên --> Xem lot --> Đăng nhập --> Đặt giá
                                              |
                                              v
Theo dõi đơn <-- Thanh toán mô phỏng <-- Thắng đấu giá
```

Các quyết định đã chốt:

- Một seller do admin vận hành, nhiều phiên đấu giá theo đợt.
- Mỗi phiên gồm nhiều lot được đặt giá độc lập. Lot là đơn vị xác định người thắng riêng.
- Hướng nội dung vintage: máy ảnh film, đồng hồ, đĩa vinyl, thiết bị âm thanh và đồ trang trí.
- Thiết kế website mới; không bắt buộc giữ các route, giao diện hay trải nghiệm game demo hiện tại.
- Có thể tái sử dụng phần engine đấu giá phù hợp, sau khi đánh giá khả năng hỗ trợ mô hình mới.
- Không cart, Buy Now hay seller portal trong phạm vi này.
- Thanh toán và vận chuyển mô phỏng; ưu tiên happy path chạy trọn vẹn.
- Analytics và recommendations đứng sau luồng mua qua đấu giá. ML là change mở rộng riêng.

Các mặc định đề xuất, cần cụ thể hóa trong proposal tương ứng:

- Cho phép nhiều phiên trùng thời gian; lot trong một phiên mở cùng lúc và đóng lần lượt theo khoảng cách cấu hình.
- MVP dùng đấu giá tăng giá công khai, đặt giá thủ công và bước giá cố định theo lot.
- Không auto-bid, reserve price, anti-sniping, Dutch hoặc sealed bidding ở giai đoạn đầu.
- Xem nội dung công khai; cần đăng nhập để bid và sử dụng watchlist.
- Mỗi lot thắng tạo một đơn; checkout từng đơn, chưa gộp thanh toán nhiều lot.
- Không sử dụng budget hoặc truth_value của game cũ làm tiền mua hàng hay dữ liệu công khai của website mới.

## 2. Định hướng thiết kế

Concept đề xuất: **Collected Stories** — nhà đấu giá trực tuyến cho những món đồ có câu chuyện.

| Thành phần | Định hướng |
|---|---|
| Nền | Ivory `#F6F1E8` |
| Chữ | Charcoal `#24211E` |
| Màu chủ đạo | Burgundy `#722F37` |
| Màu phụ | Olive `#62664D`, brass `#A38355`, dùng tiết chế |
| Typography | Tiêu đề serif; nội dung sans-serif; giá và countdown dùng tabular numerals |
| Hình ảnh | Ảnh lớn, nền trung tính, ảnh chi tiết tình trạng và dấu vết thời gian |
| Bố cục | Editorial, nhiều khoảng thở, đường kẻ mảnh, ít hiệu ứng |
| Tương tác | Khu vực bid rõ ràng, trạng thái bằng chữ và màu, focus dễ thấy |
| Mobile | Gallery, bộ lọc, thao tác bid và checkout dễ sử dụng trên điện thoại |

Màu sắc và font là định hướng ban đầu; Change 1 xác nhận lựa chọn cụ thể và kiểm tra độ tương phản. Font và asset cần có thể phục vụ cùng app để demo không phụ thuộc nguồn ngoài.

Trang lot là tâm điểm: bộ ảnh, tên, thương hiệu, niên đại, tình trạng, thông số, câu chuyện/nguồn gốc được cung cấp, giá hiện tại, bước giá, lịch sử bid, thời gian đóng và thông tin giao nhận. Không gắn nhãn xác thực khi không có thông tin giải thích tương ứng.

Dữ liệu demo đề xuất: 15–20 lot trong ba phiên có chủ đề, ví dụ *Analog Memories*, *Objects of Time*, *The Listening Room*.

## 3. Roadmap theo change

### Change 1 — `establish-vintage-design-and-site-foundation`

**Mục tiêu:** tạo nền tảng thiết kế và cấu trúc website mới.

- Chốt tokens: màu, typography, spacing, border, trạng thái và responsive behavior.
- Xây component chung: navigation, footer, button, form, lot card, status badge.
- Tạo layout storefront, tài khoản và admin.
- Thiết lập routing rõ ràng, direct links và trang 404.
- Chuẩn bị font/asset phục vụ nội bộ.
- Quyết định thay thế hoặc loại bỏ route demo cũ; `/screen` không phải yêu cầu của website mới.

**Hoàn thành khi:** khung website responsive nhất quán, điều hướng và mở trực tiếp URL hoạt động. Trang khung chưa được tính là tính năng nghiệp vụ hoàn chỉnh.

### Change 2 — `manage-vintage-lots-and-auction-events`

**Mục tiêu:** xây dữ liệu và công cụ quản lý nội dung đấu giá.

- Category cho các nhóm đồ vintage.
- Lot: ảnh, mô tả, thương hiệu, niên đại, tình trạng, thông số, câu chuyện và giao nhận.
- Phiên: tên, chủ đề, ảnh bìa, mô tả, lịch và danh sách lot.
- Admin tạo/sửa lot, xếp lot vào phiên, lưu nháp và xuất bản.
- Cấu hình lịch mở chung và thời điểm đóng lần lượt của lot.
- Seed 15–20 lot và ba phiên mẫu.
- Phân biệt thông tin công khai với cấu hình chỉ dành cho admin.

**Hoàn thành khi:** admin chuẩn bị và xuất bản được một phiên; dữ liệu còn sau restart. Lifecycle đấu giá realtime được triển khai ở Change 5.

### Change 3 — `build-auction-discovery-experience`

**Trạng thái:** đã triển khai và nghiệm thu ngày 2026-10-19. [Bằng chứng](openspec/changes/archive/2026-10-19-build-auction-discovery-experience/verification/REPORT.md) ghi nhận kiểm thử PostgreSQL, Vite và bản nhúng. Giai đoạn chỉ phản ánh lịch dự kiến; đặt giá, tài khoản/watchlist, checkout/orders, analytics/recommendations (Changes 4–8) và ML extension vẫn là công việc tương lai.

**Mục tiêu:** hoàn thiện trải nghiệm khám phá công khai.

| Route | Nội dung |
|---|---|
| `/` | Thương hiệu, phiên nổi bật, lot sắp kết thúc, category |
| `/auctions` | Phiên trong khoảng dự kiến, sắp mở và đã qua lịch dự kiến |
| `/auctions/:slug` | Câu chuyện phiên, lịch và danh sách lot |
| `/lots` | Tìm kiếm, lọc category/giai đoạn dự kiến và sắp xếp |
| `/lots/:slug` | Gallery, thông tin lot, giá, thời gian và khu vực bid |

- Dùng dữ liệu đã xuất bản từ Change 2.
- Có trạng thái loading, empty, unavailable và lỗi cơ bản.
- Hiển thị rõ tình trạng vật phẩm và phí/cách giao nhận trước khi bid.
- Khu vực bid chưa kích hoạt giao dịch cho đến Change 5.

**Hoàn thành khi:** người dùng khám phá được từ homepage đến chi tiết lot bằng dữ liệu lưu thật, trên desktop và mobile.

### Change 4 — `add-bidder-accounts-and-watchlists`

**Trạng thái:** đã triển khai và archive theo yêu cầu, còn 2/34 task nghiệm thu zoom 200% chưa xác minh. [Bản ghi archive](openspec/changes/archive/2026-10-20-add-bidder-accounts-and-watchlists/ARCHIVE.md) và [bằng chứng](openspec/changes/archive/2026-10-20-add-bidder-accounts-and-watchlists/verification/REPORT.md) giữ nguyên giới hạn này; chưa đánh dấu nghiệm thu hoàn tất.

**Mục tiêu:** thiết lập danh tính và lưu sở thích của người tham gia.

- Đăng ký, đăng nhập, đăng xuất, hồ sơ cơ bản và tài khoản demo sẵn.
- Session đăng nhập và phân quyền bidder/admin.
- Xem công khai; bid và watchlist yêu cầu đăng nhập.
- Thêm/bỏ lot khỏi watchlist và xem tại `/account/watchlist`.
- Tách tài khoản mới khỏi nickname/token/budget trò chơi cũ.
- Chưa cần xác minh email hoặc khôi phục mật khẩu.

**Hoàn thành khi:** người dùng đăng nhập và xem lại watchlist qua các lần truy cập; tài khoản thường không thực hiện được thao tác admin.

### Change 5 — `enable-timed-auction-bidding`

**Trạng thái:** đã triển khai và archive theo yêu cầu với 38/39 task. [Bản ghi archive](openspec/changes/archive/2026-10-20-enable-timed-auction-bidding/ARCHIVE.md) và [bằng chứng](openspec/changes/archive/2026-10-20-enable-timed-auction-bidding/verification/REPORT.md) giữ task 8.4 chưa hoàn tất: cần xác minh zoom trình duyệt 200% và reduced motion thực tế. Đặt giá/kết quả và trang lượt tham gia đã triển khai; chưa nghiệm thu đầy đủ. Checkout/orders vẫn là công việc tiếp theo.

**Mục tiêu:** đưa đấu giá realtime vào website mới.

- Đánh giá và tái sử dụng logic engine phù hợp.
- Hỗ trợ nhiều lot hoạt động đồng thời, thay cho một item hiện hành của game.
- Đấu giá tăng giá công khai, thao tác thủ công, bước giá theo lot.
- Cập nhật realtime giá hiện tại, lịch sử bid và trạng thái dẫn giá/bị vượt.
- Đóng lot theo thời gian server và xác định người thắng.
- `/account/bids`: đang dẫn, bị vượt và đã thắng.
- Lot không có bid kết thúc với trạng thái không bán được; chưa tự động đăng lại.

**Hoàn thành khi:** hai tài khoản cạnh tranh trên cùng lot, nhận kết quả nhất quán và xác định đúng người thắng khi hết giờ; có thể diễn tập nhiều lot đồng thời.

**Kiểm tra tối thiểu:** bid đủ mức yêu cầu, lot còn mở, danh tính hợp lệ, thứ tự cập nhật nhất quán và trạng thái kết quả được lưu. Không mở rộng thành hệ thống xử lý mọi edge case production.

### Change 6 — `add-winner-checkout-and-demo-payment`

**Mục tiêu:** người thắng có thể hoàn tất giao dịch mô phỏng.

- Tự tạo đơn chờ thanh toán từ kết quả thắng.
- Lưu snapshot thông tin lot, giá thắng và currency trên đơn.
- Trang các món thắng đang chờ thanh toán.
- Checkout từng đơn: liên hệ, địa chỉ hoặc nhận trực tiếp.
- Server tính tổng: giá thắng cộng phí giao nhận đã công bố.
- Chọn phương thức thanh toán giả lập, xác nhận thành công và hiển thị confirmation.
- Ghi rõ đây là mô phỏng; không thu thập thông tin thẻ thật.
- Chưa xử lý người thắng bỏ thanh toán, hoàn tiền hoặc gộp nhiều lot.

**Hoàn thành khi:** người thắng thanh toán và xem đơn đã thanh toán; xác nhận lặp không tạo thêm đơn hay ghi nhận thanh toán trùng.

### Change 7 — `add-order-history-and-demo-fulfillment`

**Trạng thái:** đã triển khai, 39/41 task có bằng chứng; chưa nghiệm thu đầy đủ. [Báo cáo](openspec/changes/archive/2026-10-21-add-order-history-and-demo-fulfillment/verification/REPORT.md) ghi nhận test/race/build, hai hành trình Vite/embedded, restart PostgreSQL và reset/replay demo; 7.5–7.6 còn mở. Đã archive ngày 2026-10-21 với các giới hạn này được giữ nguyên.

**Mục tiêu:** hoàn thiện phần sau mua và vận hành demo.

- `/account/orders` và trang chi tiết đơn.
- Admin xem, lọc và cập nhật trạng thái đơn.
- Giao hàng: chờ thanh toán → đã thanh toán → đang chuẩn bị → đang giao → hoàn tất.
- Nhận trực tiếp: chờ thanh toán → đã thanh toán → sẵn sàng nhận → hoàn tất.
- Timeline trạng thái; thông tin vận chuyển nhập thủ công.
- Công cụ seed/reset demo với phạm vi rõ ràng để diễn tập lại.

**Hoàn thành khi:** người mua và admin theo dõi cùng một đơn đến hoàn tất; người mua chỉ truy cập được đơn của mình.

### Change 8 — `add-auction-analytics-and-recommendations`

**Trạng thái:** đã triển khai và xác minh 40/40 task và archive ngày 2026-10-22. [Báo cáo](openspec/changes/archive/2026-10-22-add-auction-analytics-and-recommendations/verification/REPORT.md) ghi nhận PostgreSQL/test/race/build, Vite/embedded, restart/reset/replay, EN/VI, 320/375 pixels, zoom trình duyệt 200% thực tế và reduced motion hoạt động. Gợi ý theo quy tắc chưa chứng minh hiệu quả ML; các khoảng trống nghiệm thu trước đây giữ nguyên.

**Mục tiêu:** bổ sung dashboard và khám phá có liên quan sau khi happy path hoàn chỉnh.

- Dashboard: số bidder, số bid, tỷ lệ lot bán được, tổng giá thắng và tổng giá trị đã thanh toán mô phỏng.
- So sánh mức độ tham gia và kết quả giữa các phiên.
- Gợi ý lot theo category, thời kỳ hoặc watchlist; có fallback cho người dùng mới.
- Ghi nhận tương tác cần thiết: xem lot, thêm watchlist, đặt bid, thanh toán.
- Định nghĩa chỉ số rõ ràng; tách giá thắng khỏi giá trị đã thanh toán, và nhận diện dữ liệu demo.
- Dashboard hiển thị dữ liệu tổng hợp; dữ liệu đơn hàng và thông tin cá nhân vẫn được bảo vệ.

**Hoàn thành khi:** chỉ số khớp dữ liệu demo và gợi ý dẫn tới các lot phù hợp. Gợi ý theo quy tắc chưa được gọi là ML.

### Change 9 — `polish-auction-website-experience`

**Trạng thái:** đã triển khai, xác minh và archive ngày 2026-10-23; 47/47 task có bằng chứng trong [báo cáo](openspec/changes/archive/2026-10-23-polish-auction-website-experience/verification/REPORT.md). Đã sync 11 capability specs. Giao diện dùng bộ ảnh vector cục bộ gồm 39 asset, gallery phóng lớn, menu ngôn ngữ, watchlist dùng trạng thái đã xác nhận và chuyển động có reduced motion. Thanh toán/nhận hàng vẫn là mô phỏng nội bộ.

Bằng chứng mới gồm hai hành trình đấu giá → thanh toán → giao/nhận trên Vite và bản nhúng; 388 quan sát route/layout, 176 trạng thái hẹp, 20 ca form/biên nhận ở zoom trình duyệt 200% thực tế cùng reduced motion, kiểm tra quyền riêng tư và phục hồi kết quả chưa rõ. Các giới hạn/checklist archive của Changes 4–7 được giữ nguyên; bằng chứng Change 9 bổ sung coverage hiện tại, không sửa lịch sử nghiệm thu. Chi phí JS và ảnh tăng so với baseline được ghi rõ trong báo cáo.

### Mở rộng — `introduce-ml-based-lot-recommendations`

Chỉ thực hiện khi còn thời gian hoặc có yêu cầu môn học:

- Chọn mô hình theo dữ liệu thực sự có, dùng tương tác được thu thập từ Change 8.
- Huấn luyện, version model và so sánh với baseline.
- Có fallback khi thiếu dữ liệu hoặc model không khả dụng.
- Ghi rõ nếu dùng dữ liệu tổng hợp: đây là minh họa ML, chưa chứng minh hiệu quả với người dùng thực.

**Hoàn thành khi:** model thực sự tạo recommendations, có kết quả đánh giá với baseline và có thể demo ổn định.

## 4. Thứ tự và mốc bàn giao

```text
1. Design foundation
        |
        v
2. Catalog & admin
        |
        v
3. Discovery
        |
        v
4. Accounts & watchlists
        |
        v
5. Realtime bidding
        |
        v
6. Checkout & demo payment
        |
        v
7. Orders & fulfillment
        |
        v
8. Analytics & recommendations
        |
        v
   ML extension (optional)
```

| Mốc | Kết quả có thể trình diễn |
|---|---|
| Sau Change 3 | Website vintage có nội dung và trải nghiệm khám phá hoàn chỉnh |
| Sau Change 5 | Đấu giá online nhiều tài khoản, nhiều lot |
| Sau Change 7 | Khám phá → bid → thắng → thanh toán → nhận hàng mô phỏng |
| Sau Change 8 | Dashboard và recommendations bổ sung cho presentation |

Ưu tiên hoàn thành Change 1–7 trước khi đầu tư vào ML.

## 5. Ranh giới kỹ thuật và chất lượng

- Giữ single Go binary và React SPA; Change 2 thay runtime SQLite bằng PostgreSQL 18 (dịch vụ riêng). SQLite v4 chỉ còn là nguồn import offline có kiểm chứng.
- Domain mới gồm account, auction event, lot, bid, order và payment simulation. Không mặc định coi game/player/item cũ là mô hình commerce hoàn chỉnh.
- Server là nguồn quyết định giá, thời gian đóng, kết quả thắng và tổng đơn.
- Chốt đơn vị tiền tệ và cách lưu số tiền trong proposal trước khi triển khai bid/checkout. Không tái diễn giải ngầm số nguyên game cũ thành tiền commerce.
- Phân quyền phía server cho admin, bid và truy cập đơn; không chỉ ẩn nút ở frontend.
- Giữ dữ liệu chính qua restart, nhưng chưa yêu cầu kịch bản phục hồi production toàn diện.
- Kiểm tra vừa đủ cho happy path và invariant cốt lõi; không mở rộng roadmap vì các edge case không cần thiết cho demo.

Ngoài phạm vi: multi-seller, seller onboarding/payout, cart, Buy Now, payment/carrier thật, thuế đa quốc gia, hoàn tiền, tranh chấp, chống gian lận nâng cao và xử lý đầy đủ trường hợp không thanh toán.

## 6. Cách sử dụng roadmap với OpenSpec

1. Giữ tài liệu này làm định hướng và thứ tự ưu tiên.
2. Khi bắt đầu một change, dùng `$openspec-propose` với tên change tương ứng để tạo proposal, design, specs và tasks.
3. Review phạm vi, rồi dùng `$openspec-apply-change` để triển khai.
4. Verify, archive và đánh giá lại change tiếp theo dựa trên kết quả thực tế.
5. Không tạo đồng loạt tám proposal; các quyết định kỹ thuật chi tiết được chốt khi đến change liên quan.

Roadmap không phải task checklist triển khai. Việc lưu roadmap không đồng nghĩa đã tạo proposal hay hoàn thành bất kỳ change nào.
