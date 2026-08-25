# ⚡ DockPulse - Ultra Lightweight Docker & Host Control Hub

**DockPulse** là hệ thống quản lý Docker, Docker Compose Stacks và Server Host siêu nhẹ, hiện đại và bảo mật. Ứng dụng cung cấp bảng điều khiển (Dashboard) theo thời gian thực (Real-time), giám sát chi tiết mức tiêu thụ tài nguyên (CPU, RAM, Network I/O), lưu trữ lịch sử qua Cloud Supabase và hỗ trợ trợ lý thông minh chẩn đoán sự cố hệ thống.

---

## ✨ Tính Năng Nổi Bật

### 📊 1. Giám Sát Tài Nguyên Hệ Thống & Docker Engine (Realtime & Lịch Sử)
- **Host System Monitoring**: Theo dõi % CPU, RAM Memory, Swap, Disk Space, Disk I/O tốc độ đọc/ghi, Network I/O và Load Average.
- **Docker Engine Consumption**: Tổng hợp % CPU, RAM (MB và % RAM Server) và Network Traffic (📥 Rx / 📤 Tx) mà toàn bộ Docker Engine đang chiếm dụng.
- **Lưu Trữ Lịch Sử Qua Cloud (Supabase)**: Hỗ trợ chuyển đổi đồ thị xem xu hướng tài nguyên **24 Giờ Qua**, **7 Ngày Qua**, **30 Ngày Qua** với chi phí tài nguyên Host gần như bằng 0 (`0% CPU, 0 Byte Disk Write`).

### 🧩 2. Quản Lý Docker Compose Stacks
- Thống kê toàn bộ các dự án Docker Compose theo Stack (`running`, `partial`, `stopped`).
- **Stack Resource Usage**: Đo tổng mức tiêu thụ tài nguyên của từng Stack và hiển thị chi tiết chỉ số (% CPU, RAM MB/%, Net I/O) cho từng Service/Container thành phần.
- Thao tác nhanh: Start, Stop, Restart toàn bộ Compose Stack chỉ với 1 click.

### 📦 3. Quản Lý Containers, Images, Volumes & Networks
- **Containers**: Xem danh sách chi tiết (Ports mapping, IP Address, Status), theo dõi CPU/RAM/Net realtime, xem live logs stream, dọn dẹp log, thao tác Start/Stop/Restart/Pause/Remove.
- **Images**: Quản lý repository, tag, dung lượng, hỗ trợ tính năng **Prune Unused Images** dọn dẹp ảnh thừa.
- **Volumes**: Quản lý volume, mount path, dung lượng đĩa chiếm dụng, hỗ trợ **Prune Unused Volumes**.
- **Networks & Ports**: Giám sát các cổng Listening trên Host Server và quản lý các Docker Network custom.

### 💻 4. Terminal Shell Trực Tiếp Trên Web
- **Host Web Terminal**: Truy cập trực tiếp vào Terminal Bash của Host Server ngay trên trình duyệt web.
- **Container Web Terminal**: Kết nối vỏ lệnh Shell vào bất kỳ Container nào đang chạy.

### 📁 5. Trình Quản Lý Tệp (Web File Manager)
- Duyệt thư mục, tạo tệp/thư mục mới, tải lên (Upload) tệp dung lượng lớn, chỉnh sửa trực tiếp (Code Editor) và tải xuống (Download).

### 🤖 6. Smart AI Troubleshooting Assistant
- Trợ lý thông minh hỗ trợ phân tích sự cố tự động, đọc log container để đưa ra gợi ý xử lý và đánh giá sức khỏe toàn diện hệ thống (Health Audit).

---

## 🛠️ Công Nghệ Sử Dụng

- **Backend**: Go (Golang) - Ultra fast, low memory footprint (~1.5 MB RAM execution).
- **Frontend**: Vanilla HTML5, CSS3, JavaScript ES6, Chart.js - Giao diện Dark Mode sang trọng, không cần framework nặng nề.
- **Real-time Communication**: WebSockets cho Live Logs, Terminal Shell và Metrics Streaming.
- **Cloud Metrics Persistence**: Supabase REST API (PostgreSQL as a Service).

---

## 🚀 Hướng Dẫn Cài Đặt & Chạy Ứng Dụng

### 📋 1. Yêu Cầu Tiền Đề
- Linux OS (Ubuntu, Debian, CentOS, AlmaLinux, etc.)
- Go 1.13+ (để biên dịch nguồn)
- Docker Engine đã được cài đặt và đang chạy socket `/var/run/docker.sock`.

### ⚡ 2. Cài Đặt Nhanh

```bash
# 1. Clone repository
git clone https://github.com/your-username/dockpulse.git
cd dockpulse

# 2. Tạo tệp cấu hình .env từ mẫu .env.example
cp .env.example .env

# 3. Chỉnh sửa thông số tài khoản và port trong .env
nano .env

# 4. Biên dịch ứng dụng
go build -o dockpulse main.go

# 5. Chạy thử nghiệm
./dockpulse
```

---

## 🗄️ Cấu Hình Supabase Cho Lịch Sử Tài Nguyên (Tùy Chọn)

Nếu bạn muốn lưu trữ đồ thị lịch sử **24h / 7d / 30d**, hãy thiết lập Supabase theo các bước sau:

1. Tạo dự án miễn phí tại [supabase.com](https://supabase.com).
2. Vào **SQL Editor** trên Supabase và chạy đoạn script tạo bảng:

```sql
CREATE TABLE IF NOT EXISTS resource_metrics (
    id BIGSERIAL PRIMARY KEY,
    server_name VARCHAR(100) DEFAULT 'default',
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    host_cpu_percent REAL DEFAULT 0,
    host_ram_used_mb INT DEFAULT 0,
    host_ram_total_mb INT DEFAULT 0,
    host_net_rx_rate_kb REAL DEFAULT 0,
    host_net_tx_rate_kb REAL DEFAULT 0,

    docker_cpu_percent REAL DEFAULT 0,
    docker_ram_used_mb INT DEFAULT 0,
    docker_running_ctrs INT DEFAULT 0,
    docker_net_rx_mb REAL DEFAULT 0,
    docker_net_tx_mb REAL DEFAULT 0
);

-- Nếu bảng đã tồn tại từ trước, bạn chỉ cần chạy lệnh sau để thêm cột server_name:
-- ALTER TABLE resource_metrics ADD COLUMN IF NOT EXISTS server_name VARCHAR(100) DEFAULT 'default';

CREATE INDEX IF NOT EXISTS idx_resource_metrics_server_recorded_at 
ON resource_metrics (server_name, recorded_at DESC);

-- Tắt RLS để ứng dụng đẩy metrics tự động
ALTER TABLE resource_metrics DISABLE ROW LEVEL SECURITY;
```

3. Copy `SUPABASE_URL` và `SUPABASE_KEY` trong mục **Project Settings -> API** dán vào tệp `.env` (Định danh từng VPS bằng `SERVER_NAME`):

```env
SERVER_NAME=vps-01
SUPABASE_URL=https://your-project-id.supabase.co
SUPABASE_KEY=your_supabase_api_key
METRICS_PUSH_INTERVAL_SEC=300
```

---

## ⚙️ Thiết Lập Chạy Ngầm Tự Động (Systemd Service)

DockPulse đã được tích hợp tính năng **Tự Động Tạo Systemd Service 100%**:

### Cách 1: Tự động (Khuyên dùng)
- Khi bạn chạy `./dockpulse` lần đầu tiên với quyền `root` trên VPS mới, ứng dụng sẽ **tự động phát hiện, tự tạo file service `/etc/systemd/system/dockpulse.service` với đúng đường dẫn thư mục hiện tại, và tự kích hoạt chạy ngầm (`enable --now`)**.
- Hoặc bạn có thể chạy lệnh sau để tự động cài service:
```bash
./dockpulse --install-service
```

---

### Cách 2: Tạo thủ công bằng tay (Nếu muốn tùy biến)
1. Tạo tệp `/etc/systemd/system/dockpulse.service`:

```ini
[Unit]
Description=DockPulse - Ultra Lightweight Docker & Podman Compose Manager
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=root
WorkingDirectory=/root/hostcontrol
ExecStart=/root/hostcontrol/dockpulse
Restart=always
RestartSec=5
Environment=PORT=3800

[Install]
WantedBy=multi-user.target
```

2. Kích hoạt và chạy Service:

```bash
systemctl daemon-reload
systemctl enable --now dockpulse
systemctl status dockpulse
```

---

## 🔒 License

- **License**: MIT License. Free to use and modify.
