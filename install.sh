#!/usr/bin/env bash
# ==============================================================================
# ⚡ DockPulse - One-Line Smart Installer Script
# Ultra Lightweight Docker & Podman Compose Manager with Security & Fail2ban
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
echo -e "${BOLD}🚀 Khởi chạy trình cài đặt thông minh DockPulse (Host & Container Control Hub)${NC}\n"

# 1. Check Root Privileges
if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}❌ Vui lòng chạy lệnh cài đặt với quyền root (sudo bash install.sh)${NC}"
  exit 1
fi

INSTALL_DIR="/opt/dockpulse"
PORT=3800
WITH_FAIL2BAN=""
ENGINE_CHOICE=""
NON_INTERACTIVE="no"

# Parse command line flags
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --with-fail2ban) WITH_FAIL2BAN="yes" ;;
        --no-fail2ban) WITH_FAIL2BAN="no" ;;
        --engine=*) ENGINE_CHOICE="${1#*=}" ;;
        --port=*) PORT="${1#*=}" ;;
        --dir=*) INSTALL_DIR="${1#*=}" ;;
        -y|--yes|--unattended) NON_INTERACTIVE="yes" ;;
        *) echo "Unknown option: $1" ;;
    esac
    shift
done

# Safe input prompt supporting pipeline execution (curl ... | sudo bash)
prompt_user() {
    local prompt_msg="$1"
    local default_val="$2"
    local user_val=""
    if [ "$NON_INTERACTIVE" = "yes" ]; then
        echo "$default_val"
        return
    fi
    if [ -t 0 ]; then
        read -p "$prompt_msg" user_val
    elif [ -e /dev/tty ]; then
        read -p "$prompt_msg" user_val < /dev/tty
    else
        user_val="$default_val"
    fi
    echo "${user_val:-$default_val}"
}

# 2. Detect OS Distribution & Hardware Specs
OS_NAME="unknown"
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_NAME=$ID
fi
echo -e "📦 Hệ điều hành: ${GREEN}${PRETTY_NAME:-$OS_NAME}${NC}"

# Detect Total RAM
TOTAL_MEM_KB=$(awk '/MemTotal/ {print $2}' /proc/meminfo 2>/dev/null || echo 0)
TOTAL_MEM_MB=$((TOTAL_MEM_KB / 1024))
TOTAL_MEM_GB=$(awk "BEGIN {printf \"%.1f\", $TOTAL_MEM_MB / 1024}")

echo -e "🧠 Dung lượng RAM phát hiện: ${CYAN}${TOTAL_MEM_GB} GB${NC} (${TOTAL_MEM_MB} MB)"

# Determine Recommended Engine based on RAM:
# Under 4GB (< 4096MB) -> Podman (daemonless, ultra lightweight, saves RAM)
# 4GB or more (>= 4096MB) -> Docker (standard ecosystem)
if [ "$TOTAL_MEM_MB" -lt 4096 ]; then
    RECOMMENDED_ENGINE="podman"
    RECOMMENDED_DESC="Podman (Siêu nhẹ, không tốn daemon RAM nền - Tối ưu cho VPS dưới 4GB RAM)"
    DEFAULT_OPT="1"
else
    RECOMMENDED_ENGINE="docker"
    RECOMMENDED_DESC="Docker Engine (Tương thích phổ biến rộng rãi - Phù hợp cho máy từ 4GB RAM trở lên)"
    DEFAULT_OPT="2"
fi

# Detect existing installations
DOCKER_INSTALLED="no"
PODMAN_INSTALLED="no"
command -v docker >/dev/null 2>&1 && DOCKER_INSTALLED="yes"
command -v podman >/dev/null 2>&1 && PODMAN_INSTALLED="yes"

if [ "$DOCKER_INSTALLED" = "yes" ] || [ "$PODMAN_INSTALLED" = "yes" ]; then
    echo -e "🔍 Hiện trạng Container Engine:"
    [ "$DOCKER_INSTALLED" = "yes" ] && echo -e "   - Docker Engine: ${GREEN}Đã cài đặt${NC}"
    [ "$PODMAN_INSTALLED" = "yes" ] && echo -e "   - Podman Engine: ${GREEN}Đã cài đặt${NC}"
fi

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

# 4. Container Engine Selection & Auto Setup
echo ""
echo -e "${YELLOW}⚙️ [Cấu Hình Container Engine]${NC}"
echo -e "   - Đề xuất tối ưu cho phần cứng máy chủ: ${BOLD}${RECOMMENDED_DESC}${NC}"

if [ -z "$ENGINE_CHOICE" ]; then
    echo -e "   Vui lòng chọn Engine quản lý container bạn muốn sử dụng:"
    echo -e "     ${BOLD}1) Podman${NC}       (Mặc định cho máy < 4GB RAM - Tự động kích hoạt socket API)"
    echo -e "     ${BOLD}2) Docker${NC}       (Mặc định cho máy >= 4GB RAM)"
    echo -e "     ${BOLD}3) Bỏ qua${NC}       (Giữ nguyên engine hiện tại hoặc tự cấu hình sau)"
    
    user_engine_sel=$(prompt_user "👉 Chọn [1/2/3] (Mặc định là [${DEFAULT_OPT}]): " "$DEFAULT_OPT")
    case "$user_engine_sel" in
        1) ENGINE_CHOICE="podman" ;;
        2) ENGINE_CHOICE="docker" ;;
        3) ENGINE_CHOICE="skip" ;;
        *) 
            if [ "$DEFAULT_OPT" = "1" ]; then
                ENGINE_CHOICE="podman"
            else
                ENGINE_CHOICE="docker"
            fi
            ;;
    esac
fi

# 5. Fail2ban Selection
if [ -z "$WITH_FAIL2BAN" ]; then
    echo ""
    echo -e "${YELLOW}🛡️ [Khuyến Nghị Bảo Mật] Bạn có muốn tự động cài đặt & cấu hình Fail2ban?${NC}"
    echo -e "   - Tự động chặn IP brute-force SSH (cổng 22) và bot quét Web server."
    echo -e "   - Tự động whitelist IP của bạn (${CLIENT_IP:-'127.0.0.1'}) để tránh bị khóa nhầm."
    user_f2b_choice=$(prompt_user "👉 Cài đặt Fail2ban ngay bây giờ? [Y/n]: " "Y")
    if [[ "$user_f2b_choice" =~ ^[Yy]$ ]]; then
        WITH_FAIL2BAN="yes"
    else
        WITH_FAIL2BAN="no"
    fi
fi

# 6. Install System Dependencies & Go Compiler
echo -e "\n⏳ Đang cập nhật gói hệ thống & cài đặt phụ thuộc cần thiết..."
if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq curl tar gzip git systemd rsyslog ca-certificates >/dev/null 2>&1 || true
    if [ "$WITH_FAIL2BAN" = "yes" ]; then
        echo -e "🛡️ Đang cài đặt Fail2ban qua APT..."
        apt-get install -y -qq fail2ban >/dev/null 2>&1 || true
    fi
    if ! command -v go >/dev/null 2>&1; then
        echo -e "⚙️ Đang cài đặt trình biên dịch Go (Golang)..."
        apt-get install -y -qq golang-go >/dev/null 2>&1 || true
    fi
elif command -v dnf >/dev/null 2>&1; then
    dnf install -y -q curl tar gzip git systemd ca-certificates >/dev/null 2>&1 || true
    if [ "$WITH_FAIL2BAN" = "yes" ]; then
        echo -e "🛡️ Đang cài đặt Fail2ban qua DNF..."
        dnf install -y -q epel-release >/dev/null 2>&1 || true
        dnf install -y -q fail2ban >/dev/null 2>&1 || true
    fi
    if ! command -v go >/dev/null 2>&1; then
        echo -e "⚙️ Đang cài đặt trình biên dịch Go (Golang)..."
        dnf install -y -q golang >/dev/null 2>&1 || true
    fi
elif command -v yum >/dev/null 2>&1; then
    yum install -y -q curl tar gzip git systemd ca-certificates >/dev/null 2>&1 || true
    if [ "$WITH_FAIL2BAN" = "yes" ]; then
        echo -e "🛡️ Đang cài đặt Fail2ban qua YUM..."
        yum install -y -q epel-release >/dev/null 2>&1 || true
        yum install -y -q fail2ban >/dev/null 2>&1 || true
    fi
    if ! command -v go >/dev/null 2>&1; then
        echo -e "⚙️ Đang cài đặt trình biên dịch Go (Golang)..."
        yum install -y -q golang >/dev/null 2>&1 || true
    fi
fi

# Fallback check for Go: if still not available, download official minimal Go binary archive
if ! command -v go >/dev/null 2>&1; then
    echo -e "⬇️ Đang tải bản phân phối Go chính thức tự động..."
    ARCH="$(uname -m)"
    GO_ARCH="amd64"
    [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ] && GO_ARCH="arm64"
    GO_TMP="/tmp/go_install"
    mkdir -p "$GO_TMP"
    GO_TAR="go1.22.6.linux-${GO_ARCH}.tar.gz"
    if curl -fsSL "https://go.dev/dl/${GO_TAR}" -o "$GO_TMP/${GO_TAR}"; then
        tar -C /usr/local -xzf "$GO_TMP/${GO_TAR}" >/dev/null 2>&1 || true
        export PATH=$PATH:/usr/local/go/bin
        ln -sf /usr/local/go/bin/go /usr/bin/go 2>/dev/null || true
    fi
    rm -rf "$GO_TMP"
fi

# 7. Execute Engine Setup
if [ "$ENGINE_CHOICE" = "podman" ]; then
    echo -e "\n🦭 [Cài Đặt & Cấu Hình Podman Engine]"
    if ! command -v podman >/dev/null 2>&1; then
        echo -e "⏳ Đang cài đặt gói Podman..."
        if command -v apt-get >/dev/null 2>&1; then
            apt-get install -y -qq podman >/dev/null 2>&1 || true
        elif command -v dnf >/dev/null 2>&1; then
            dnf install -y -q podman >/dev/null 2>&1 || true
        elif command -v yum >/dev/null 2>&1; then
            yum install -y -q podman >/dev/null 2>&1 || true
        fi
    fi

    # Auto configure Podman Socket API (Essential for DockPulse)
    echo -e "⚙️ Tự động kích hoạt dịch vụ Podman Socket (/run/podman/podman.sock)..."
    systemctl daemon-reload >/dev/null 2>&1 || true
    systemctl enable podman.socket >/dev/null 2>&1 || true
    systemctl restart podman.socket >/dev/null 2>&1 || true

    mkdir -p /run/podman /var/run
    # Create compatibility symlink if /var/run/docker.sock does not exist or points to a non-existent file
    if [ ! -e /var/run/docker.sock ] || [ -L /var/run/docker.sock ]; then
        ln -sf /run/podman/podman.sock /var/run/docker.sock
    fi

    if systemctl is-active --quiet podman.socket || [ -S /run/podman/podman.sock ]; then
        echo -e "${GREEN}✅ Podman Socket đã sẵn sàng: /run/podman/podman.sock${NC}"
    else
        echo -e "${YELLOW}⚠️ Đang khởi tạo lại podman.socket qua systemctl...${NC}"
        systemctl start podman.socket >/dev/null 2>&1 || true
    fi

elif [ "$ENGINE_CHOICE" = "docker" ]; then
    echo -e "\n🐳 [Cài Đặt & Cấu Hình Docker Engine]"
    if ! command -v docker >/dev/null 2>&1; then
        echo -e "⏳ Đang cài đặt Docker Engine chính thức..."
        if ! curl -fsSL https://get.docker.com | sh >/dev/null 2>&1; then
            if command -v apt-get >/dev/null 2>&1; then
                apt-get install -y -qq docker.io >/dev/null 2>&1 || true
            elif command -v dnf >/dev/null 2>&1; then
                dnf install -y -q docker >/dev/null 2>&1 || true
            elif command -v yum >/dev/null 2>&1; then
                yum install -y -q docker >/dev/null 2>&1 || true
            fi
        fi
    fi

    echo -e "⚙️ Kích hoạt dịch vụ Docker..."
    systemctl daemon-reload >/dev/null 2>&1 || true
    systemctl enable docker >/dev/null 2>&1 || true
    systemctl restart docker >/dev/null 2>&1 || true

    if systemctl is-active --quiet docker || [ -S /var/run/docker.sock ]; then
        echo -e "${GREEN}✅ Docker Engine đã sẵn sàng: /var/run/docker.sock${NC}"
    else
        echo -e "${YELLOW}⚠️ Vui lòng kiểm tra lại dịch vụ Docker (systemctl status docker)${NC}"
    fi
else
    echo -e "⏭️ Bỏ qua bước cài đặt engine container theo yêu cầu."
fi

# 8. Configure Fail2ban if requested
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

# 9. Setup DockPulse Application
echo -e "\n📁 Thiết lập thư mục ứng dụng tại: ${CYAN}${INSTALL_DIR}${NC}"
mkdir -p "$INSTALL_DIR"

CURRENT_DIR="$(pwd)"
if [ -n "${BASH_SOURCE[0]}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

if [ -f "$CURRENT_DIR/dockpulse" ] && [ -d "$CURRENT_DIR/web" ]; then
    echo -e "📦 Sử dụng bản dựng cục bộ từ: ${CURRENT_DIR}"
    cp -r "$CURRENT_DIR/dockpulse" "$INSTALL_DIR/"
    cp -r "$CURRENT_DIR/web" "$INSTALL_DIR/"
    [ -f "$CURRENT_DIR/config.json" ] && cp "$CURRENT_DIR/config.json" "$INSTALL_DIR/"
    [ -f "$CURRENT_DIR/.env" ] && cp "$CURRENT_DIR/.env" "$INSTALL_DIR/"
    [ -f "$CURRENT_DIR/.env.example" ] && [ ! -f "$INSTALL_DIR/.env" ] && cp "$CURRENT_DIR/.env.example" "$INSTALL_DIR/.env"
else
    echo -e "⬇️ Đang tải mã nguồn DockPulse mới nhất từ GitHub..."
    BUILD_TMP=$(mktemp -d /tmp/dockpulse_src_XXXXXX)
    TAR_URL="https://github.com/tientaisv/hostpanel/archive/refs/heads/main.tar.gz"
    
    if curl -fsSL "$TAR_URL" | tar -xz -C "$BUILD_TMP" --strip-components=1; then
        echo -e "⚙️ Đang biên dịch DockPulse binary bằng Go..."
        cd "$BUILD_TMP"
        if command -v go >/dev/null 2>&1; then
            go build -o dockpulse main.go
            cp "$BUILD_TMP/dockpulse" "$INSTALL_DIR/"
            cp -r "$BUILD_TMP/web" "$INSTALL_DIR/"
            [ -f "$BUILD_TMP/config.json" ] && cp "$BUILD_TMP/config.json" "$INSTALL_DIR/"
            [ -f "$BUILD_TMP/.env.example" ] && [ ! -f "$INSTALL_DIR/.env" ] && cp "$BUILD_TMP/.env.example" "$INSTALL_DIR/.env"
            cd "$CURRENT_DIR"
            rm -rf "$BUILD_TMP"
            echo -e "${GREEN}✅ Biên dịch và triển khai DockPulse thành công!${NC}"
        else
            cd "$CURRENT_DIR"
            rm -rf "$BUILD_TMP"
            echo -e "${RED}❌ Không thể biên dịch: Thiếu trình biên dịch Go (Golang).${NC}"
            exit 1
        fi
    else
        rm -rf "$BUILD_TMP"
        echo -e "${RED}❌ Không thể tải mã nguồn từ GitHub. Vui lòng kiểm tra kết nối mạng.${NC}"
        exit 1
    fi
fi

# Ensure executable permissions
chmod +x "$INSTALL_DIR/dockpulse"

# 10. Create and Configure Systemd Service
echo -e "⚙️ Tạo và kích hoạt Systemd Service (dockpulse.service)..."
cat << EOF > /etc/systemd/system/dockpulse.service
[Unit]
Description=DockPulse - Ultra Lightweight Docker & Podman Compose Manager
After=network.target docker.service podman.service podman.socket
Wants=docker.service podman.service podman.socket

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/dockpulse
Restart=always
RestartSec=5
LimitNOFILE=65536
Environment=PORT=$PORT

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable dockpulse
systemctl restart dockpulse

# 11. Verification & Information Summary
PUBLIC_IP=$(curl -s -m 3 ifconfig.me 2>/dev/null || curl -s -m 3 api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')

echo -e "\n${GREEN}${BOLD}🎉 CÀI ĐẶT DOCKPULSE THÀNH CÔNG!${NC}"
echo -e "=========================================================="
echo -e "🌐 Truy cập Web UI:   ${CYAN}${BOLD}http://${PUBLIC_IP}:${PORT}${NC}"
echo -e "📂 Thư mục cài đặt:   ${INSTALL_DIR}"
echo -e "⚙️ Dịch vụ quản lý:   systemctl status dockpulse"
if [ "$ENGINE_CHOICE" = "podman" ]; then
    echo -e "🦭 Container Engine:  ${PURPLE}Podman (Socket API: /run/podman/podman.sock)${NC}"
elif [ "$ENGINE_CHOICE" = "docker" ]; then
    echo -e "🐳 Container Engine:  ${BLUE}Docker Engine (Socket: /var/run/docker.sock)${NC}"
fi
if [ "$WITH_FAIL2BAN" = "yes" ]; then
    echo -e "🛡️ Trạng thái bảo vệ: ${GREEN}Fail2ban Đang Hoạt Động (Bảo vệ SSH & Nginx)${NC}"
fi
echo -e "=========================================================="
echo -e "💡 Mẹo: DockPulse hỗ trợ quản lý cả Podman và Docker. Bạn có thể theo dõi tài nguyên, quản lý container, compose stacks và terminal trực tiếp trên Web Dashboard."
