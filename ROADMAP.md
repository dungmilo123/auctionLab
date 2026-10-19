# AuctionLab — Vintage Auction Website Roadmap

Ngày lập: 2026-10-19.

Trạng thái: định hướng đã thống nhất trong trao đổi; chưa tạo proposal hoặc triển khai các change bên dưới. Tên change là tên dự kiến để sử dụng khi bắt đầu từng bước.

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

**Mục tiêu:** hoàn thiện trải nghiệm khám phá công khai.

| Route dự kiến | Nội dung |
|---|---|
| `/` | Thương hiệu, phiên nổi bật, lot sắp kết thúc, category |
| `/auctions` | Phiên đang diễn ra, sắp mở và đã kết thúc |
| `/auctions/:slug` | Câu chuyện phiên, lịch và danh sách lot |
| `/lots` | Tìm kiếm, lọc category/trạng thái và sắp xếp |
| `/lots/:slug` | Gallery, thông tin lot, giá, thời gian và khu vực bid |

- Dùng dữ liệu đã xuất bản từ Change 2.
- Có trạng thái loading, empty, unavailable và lỗi cơ bản.
- Hiển thị rõ tình trạng vật phẩm và phí/cách giao nhận trước khi bid.
- Khu vực bid chưa kích hoạt giao dịch cho đến Change 5.

**Hoàn thành khi:** người dùng khám phá được từ homepage đến chi tiết lot bằng dữ liệu lưu thật, trên desktop và mobile.

### Change 4 — `add-bidder-accounts-and-watchlists`

**Mục tiêu:** thiết lập danh tính và lưu sở thích của người tham gia.

- Đăng ký, đăng nhập, đăng xuất, hồ sơ cơ bản và tài khoản demo sẵn.
- Session đăng nhập và phân quyền bidder/admin.
- Xem công khai; bid và watchlist yêu cầu đăng nhập.
- Thêm/bỏ lot khỏi watchlist và xem tại `/account/watchlist`.
- Tách tài khoản mới khỏi nickname/token/budget trò chơi cũ.
- Chưa cần xác minh email hoặc khôi phục mật khẩu.

**Hoàn thành khi:** người dùng đăng nhập và xem lại watchlist qua các lần truy cập; tài khoản thường không thực hiện được thao tác admin.

### Change 5 — `enable-timed-auction-bidding`

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

**Mục tiêu:** hoàn thiện phần sau mua và vận hành demo.

- `/account/orders` và trang chi tiết đơn.
- Admin xem, lọc và cập nhật trạng thái đơn.
- Giao hàng: chờ thanh toán → đã thanh toán → đang chuẩn bị → đang giao → hoàn tất.
- Nhận trực tiếp: chờ thanh toán → đã thanh toán → sẵn sàng nhận → hoàn tất.
- Timeline trạng thái; thông tin vận chuyển nhập thủ công.
- Công cụ seed/reset demo với phạm vi rõ ràng để diễn tập lại.

**Hoàn thành khi:** người mua và admin theo dõi cùng một đơn đến hoàn tất; người mua chỉ truy cập được đơn của mình.

### Change 8 — `add-auction-analytics-and-recommendations`

**Mục tiêu:** bổ sung dashboard và khám phá có liên quan sau khi happy path hoàn chỉnh.

- Dashboard: số bidder, số bid, tỷ lệ lot bán được, tổng giá thắng và tổng giá trị đã thanh toán mô phỏng.
- So sánh mức độ tham gia và kết quả giữa các phiên.
- Gợi ý lot theo category, thời kỳ hoặc watchlist; có fallback cho người dùng mới.
- Ghi nhận tương tác cần thiết: xem lot, thêm watchlist, đặt bid, thanh toán.
- Định nghĩa chỉ số rõ ràng; tách giá thắng khỏi giá trị đã thanh toán, và nhận diện dữ liệu demo.
- Dashboard hiển thị dữ liệu tổng hợp; dữ liệu đơn hàng và thông tin cá nhân vẫn được bảo vệ.

**Hoàn thành khi:** chỉ số khớp dữ liệu demo và gợi ý dẫn tới các lot phù hợp. Gợi ý theo quy tắc chưa được gọi là ML.

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

- Giữ hướng single Go binary, React SPA và SQLite làm điểm xuất phát; proposal từng change xác định thay đổi cụ thể nếu cần.
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
