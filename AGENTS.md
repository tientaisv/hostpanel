# DockPulse (HostControl) Development Guidelines

## Release & Versioning Policy
Trước khi thực hiện `git push` lên GitHub remote, LUÔN LUÔN thực hiện tuần tự:
1. Cập nhật `CurrentVersion` trong `pkg/updater/updater.go` lên phiên bản mới (`vX.Y.Z`).
2. Biên dịch lại binary: `go build -o dockpulse main.go` và khởi động lại dịch vụ nếu đang chạy (`systemctl restart dockpulse`).
3. Tạo git commit và git tag tương ứng: `git tag -a vX.Y.Z -m "Release vX.Y.Z: <mô tả>"`
4. Push cả commit và tag lên repository: `git push origin main && git push origin vX.Y.Z` (hoặc `git push origin --tags`).
