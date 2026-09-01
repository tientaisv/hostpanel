# Quy Định Versioning & Git Release

BẮT BUỘC TUÂN THỦ TRƯỚC KHI PUSH GIT:

Mỗi khi hoàn thành tính năng mới, sửa lỗi, hoặc chuẩn bị `git push` lên repository:

1. **Cập nhật Phiên bản Product (`pkg/updater/updater.go`)**:
   - Tăng `CurrentVersion` tương ứng theo chuẩn Semantic Versioning (`vMAJOR.MINOR.PATCH`).
   - Ví dụ: `v1.5.0` -> `v1.6.0` (tính năng mới) hoặc `v1.6.1` (sửa lỗi).

2. **Rebuild Binary & Khởi động lại Service**:
   - Chạy `go build -o dockpulse main.go`
   - Restart service: `systemctl restart dockpulse` (hoặc kiểm tra tính hợp lệ của binary).

3. **Tạo Git Commit & Tag Phiên Bản**:
   - Commit các thay đổi với thông điệp rõ ràng: `git commit -m "feat: ..."`
   - Tạo tag phiên bản tương ứng: `git tag -a vX.Y.Z -m "Release vX.Y.Z: <mô tả tóm tắt tính năng>"`

4. **Push Commit và Tag lên Git Remote**:
   - Chạy `git push origin main`
   - Chạy `git push origin vX.Y.Z` (hoặc `git push origin --tags`)
