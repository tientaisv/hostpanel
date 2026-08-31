#!/usr/bin/env bash
# ==============================================================================
# ⚡ DockPulse - One-Line Public Installer Script
# Lightweight Docker & Host Control Hub with Integrated Security & Fail2ban
# ==============================================================================

set -e

# Color definitions
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${CYAN}${BOLD}"
cat << "EOF"
  ____             _     ____        _           
 |  _ \  ___   ___| | __|  _ \ _   _| |___  ___  
 | | | |/ _ \ / __| |/ /| |_) | | | | / __|/ _ \ 
 | |_| | (_) | (__|   < |  __/| |_| | \__ \  __/ 
 |____/ \___/ \___|_|\_\|_|    \__,_|_|___/\___| 
EOF
echo -e "${NC}"
echo -e "${BOLD}🚀 Khởi chạy trình cài đặt DockPulse & Trung tâm Quản trị Máy chủ${NC}\n"

# 1. Check Root Privileges
if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}❌ Vui lòng chạy lệnh cài đặt với quyền root (sudo bash install.sh)${NC}"
  exit 1
fi

INSTALL_DIR="/opt/dockpulse"
PORT=3800
WITH_FAIL2BAN=""

# Parse command line flags
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --with-fail2ban) WITH_FAIL2BAN="yes" ;;
        --no-fail2ban) WITH_FAIL2BAN="no" ;;
        --port=*) PORT="${1#*=}" ;;
        --dir=*) INSTALL_DIR="${1#*=}" ;;
        *) echo "Unknown option: $1" ;;
    esac
    shift
done

# 2. Detect OS Distribution
OS_NAME="unknown"
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_NAME=$ID
fi
echo -e "📦 Hệ điều hành phát hiện: ${GREEN}${PRETTY_NAME:-$OS_NAME}${NC}"

# 3. Detect Admin/Client IP for Safe Whitelisting
CLIENT_IP=""
if [ -n "$SSH_CLIENT" ]; then
    CLIENT_IP=$(echo "$SSH_CLIENT" | awk '{print $1}')
elif [ -n "$SSH_CONNECTION" ]; then
    CLIENT_IP=$(echo "$SSH_CONNECTION" | awk '{print $1}')
fi

if [ -n "$CLIENT_IP" ]; then
    echo -e "🌐 Địa chỉ IP quản trị của bạn: ${CYAN}${CLIENT_IP}${NC} (sẽ được tự động Whitelist an toàn)"
fi

# 4. Fail2ban Selection
if [ -z "$WITH_FAIL2BAN" ]; then
    echo ""
    echo -e "${YELLOW}🛡️ [Khuyến Nghị Bảo Mật] Bạn có muốn tự động cài đặt & cấu hình Fail2ban?${NC}"
    echo -e "   - Tự động chặn IP brute-force SSH (cổng 22) và bot quét Web server."
    echo -e "   - Tự động whitelist IP của bạn (${CLIENT_IP:-'127.0.0.1'}) để tránh bị khóa nhầm."
    read -p "👉 Cài đặt Fail2ban ngay bây giờ? [Y/n]: " user_f2b_choice
    user_f2b_choice=${user_f2b_choice:-Y}
    if [[ "$user_f2b_choice" =~ ^[Yy]$ ]]; then
        WITH_FAIL2BAN="yes"
    else
        WITH_FAIL2BAN="no"
    fi
fi

# 5. Install Dependencies & Fail2ban (if selected)
echo -e "\n⏳ Đang cập nhật gói hệ thống & cài đặt phụ thuộc..."
if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq curl tar gzip systemd rsyslog >/dev/null 2>&1 || true
    if [ "$WITH_FAIL2BAN" = "yes" ]; then
        echo -e "🛡️ Đang cài đặt Fail2ban qua APT..."
        apt-get install -y -qq fail2ban >/dev/null 2>&1
    fi
elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q curl tar gzip systemd >/dev/null 2>&1 || true
    if [ "$WITH_FAIL2BAN" = "yes" ]; then
        echo -e "🛡️ Đang cài đặt Fail2ban qua DNF..."
        dnf install -y -q epel-release >/dev/null 2>&1 || true
        dnf install -y -q fail2ban >/dev/null 2>&1
    fi
elif command -v yum >/dev/null 2>&1; then
    yum install -y -q curl tar gzip systemd >/dev/null 2>&1 || true
    if [ "$WITH_FAIL2BAN" = "yes" ]; then
        echo -e "🛡️ Đang cài đặt Fail2ban qua YUM..."
        yum install -y -q epel-release >/dev/null 2>&1 || true
        yum install -y -q fail2ban >/dev/null 2>&1
    fi
fi

# 6. Configure Fail2ban if requested
if [ "$WITH_FAIL2BAN" = "yes" ] && command -v fail2ban-client >/dev/null 2>&1; then
    echo -e "⚙️ Đang cấu hình /etc/fail2ban/jail.local..."
    WHITELIST="127.0.0.1/8 ::1 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16"
    if [ -n "$CLIENT_IP" ]; then
        WHITELIST="$WHITELIST $CLIENT_IP"
    fi

    cat << EOF > /etc/fail2ban/jail.local
[DEFAULT]
ignoreip = $WHITELIST
bantime = 1h
findtime = 10m
maxretry = 5
bantime.increment = true
bantime.factor = 1
backend = systemd

[sshd]
enabled = true
port = 22
mode = aggressive
logpath = %(sshd_log)s
backend = %(sshd_backend)s
maxretry = 5
findtime = 10m
bantime = 1h

[nginx-http-auth]
enabled = true
port = http,https
logpath = /var/log/nginx/error.log

[nginx-botsearch]
enabled = true
port = http,https
logpath = /var/log/nginx/access.log
maxretry = 2
findtime = 10m
bantime = 24h

[nginx-bad-request]
enabled = true
port = http,https
logpath = /var/log/nginx/access.log
maxretry = 5
findtime = 10m
bantime = 1h
EOF

    systemctl daemon-reload >/dev/null 2>&1 || true
    systemctl enable fail2ban >/dev/null 2>&1 || true
    systemctl restart fail2ban >/dev/null 2>&1 || true
    echo -e "${GREEN}✅ Đã kích hoạt Fail2ban thành công!${NC}"
fi

# 7. Setup DockPulse Application
echo -e "\n📁 Thiết lập thư mục ứng dụng tại: ${CYAN}${INSTALL_DIR}${NC}"
mkdir -p "$INSTALL_DIR"

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$CURRENT_DIR/dockpulse" ] && [ -d "$CURRENT_DIR/web" ]; then
    # Local installation from source folder
    cp -r "$CURRENT_DIR/dockpulse" "$INSTALL_DIR/"
    cp -r "$CURRENT_DIR/web" "$INSTALL_DIR/"
    [ -f "$CURRENT_DIR/config.json" ] && cp "$CURRENT_DIR/config.json" "$INSTALL_DIR/"
    [ -f "$CURRENT_DIR/.env" ] && cp "$CURRENT_DIR/.env" "$INSTALL_DIR/"
else
    echo -e "⬇️ Đang tải bản phát hành DockPulse mới nhất..."
    # Download latest release logic here if hosted on GitHub Releases
fi

# 8. Create Systemd Service
echo -e "⚙️ Tạo và kích hoạt Systemd Service (dockpulse.service)..."
cat << EOF > /etc/systemd/system/dockpulse.service
[Unit]
Description=DockPulse - Ultra Lightweight Docker & Host Control Hub
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/dockpulse
Restart=always
RestartSec=5
Environment=PORT=$PORT

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now dockpulse

# 9. Get Public IP
PUBLIC_IP=$(curl -s -m 3 ifconfig.me 2>/dev/null || curl -s -m 3 api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')

echo -e "\n${GREEN}${BOLD}🎉 CÀI ĐẶT DOCKPULSE THÀNH CÔNG!${NC}"
echo -e "=========================================================="
echo -e "🌐 Truy cập Web UI:   ${CYAN}${BOLD}http://${PUBLIC_IP}:${PORT}${NC}"
echo -e "📂 Thư mục cài đặt:   ${INSTALL_DIR}"
echo -e "⚙️ Dịch vụ quản lý:   systemctl status dockpulse"
if [ "$WITH_FAIL2BAN" = "yes" ]; then
    echo -e "🛡️ Trạng thái bảo vệ: ${GREEN}Fail2ban Đang Hoạt Động (Bảo vệ SSH & Nginx)${NC}"
fi
echo -e "=========================================================="
echo -e "💡 Mẹo: Bạn có thể quản lý, xem IP bị cấm và gỡ chặn trực tiếp tại mục ${BOLD}Security & Alerts${NC} trên Web Dashboard."
