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
DIM='\033[2m'
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
ADMIN_USER=""
ADMIN_PASS=""
NON_INTERACTIVE="no"

# Parse command line flags
while [[ "$#" -gt 0 ]]; do
    case $1 in
        --with-fail2ban) WITH_FAIL2BAN="yes" ;;
        --no-fail2ban) WITH_FAIL2BAN="no" ;;
        --engine=*) ENGINE_CHOICE="${1#*=}" ;;
        --port=*) PORT="${1#*=}" ;;
        --dir=*) INSTALL_DIR="${1#*=}" ;;
        --user=*) ADMIN_USER="${1#*=}" ;;
        --pass=*) ADMIN_PASS="${1#*=}" ;;
        -y|--yes|--unattended) NON_INTERACTIVE="yes" ;;
        *) echo "Unknown option: $1" ;;
    esac
    shift
done

# Safe input prompt supporting pipeline execution (curl ... | sudo bash)
prompt_user() {
    local prompt_msg="$1"
    local default_val="$2"
    local is_secret="${3:-no}"
    local user_val=""

    if [ "$NON_INTERACTIVE" = "yes" ]; then
        echo "$default_val"
        return
    fi

    if [ -t 0 ]; then
        if [ "$is_secret" = "yes" ]; then
            read -s -p "$prompt_msg" user_val
            echo "" >&2
        else
            read -p "$prompt_msg" user_val
        fi
    elif [ -r /dev/tty ] && [ -w /dev/tty ]; then
        printf "%b" "$prompt_msg" > /dev/tty
        if [ "$is_secret" = "yes" ]; then
            read -s user_val < /dev/tty
            echo "" > /dev/tty
        else
            read user_val < /dev/tty
        fi
    else
        user_val="$default_val"
    fi
    echo "${user_val:-$default_val}"
}

# Spinner function to run tasks with progress animation
run_with_spinner() {
    local title="$1"
    shift
    local log_file="/tmp/dockpulse_step_$$.log"
    "$@" > "$log_file" 2>&1 &
    local pid=$!
    local spin='-\|/'
    local i=0
    printf "  ⏳ %s..." "$title"
    while kill -0 $pid 2>/dev/null; do
        i=$(( (i+1) % 4 ))
        printf "\r  \033[1;33m[%c]\033[0m %s..." "${spin:$i:1}" "$title"
        sleep 0.2
    done
    wait $pid
    local exit_code=$?
    if [ $exit_code -eq 0 ]; then
        printf "\r  \033[0;32m[✓]\033[0m %s... \033[0;32mXong!\033[0m\n" "$title"
        rm -f "$log_file"
        return 0
    else
        printf "\r  \033[0;31m[✗]\033[0m %s... \033[0;31mThất bại!\033[0m\n" "$title"
        echo -e "${RED}Chi tiết lỗi:${NC}"
        tail -n 25 "$log_file"
        rm -f "$log_file"
        return $exit_code
    fi
}

# ==============================================================================
# [1/6] 15% - Phát hiện môi trường hệ thống & Phần cứng
# ==============================================================================
echo -e "\n${CYAN}${BOLD}[1/6] 15% 🔍 Phát hiện môi trường hệ thống & Phần cứng...${NC}"

OS_NAME="unknown"
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_NAME=$ID
fi
echo -e "  📦 Hệ điều hành: ${GREEN}${PRETTY_NAME:-$OS_NAME}${NC}"

TOTAL_MEM_KB=$(awk '/MemTotal/ {print $2}' /proc/meminfo 2>/dev/null || echo 0)
TOTAL_MEM_MB=$((TOTAL_MEM_KB / 1024))
TOTAL_MEM_GB=$(awk "BEGIN {printf \"%.1f\", $TOTAL_MEM_MB / 1024}")
echo -e "  🧠 Dung lượng RAM: ${CYAN}${TOTAL_MEM_GB} GB${NC} (${TOTAL_MEM_MB} MB)"

# Determine Recommended Engine based on RAM:
# Under 4GB (< 4096MB) -> Podman (daemonless, saves RAM)
# 4GB or more (>= 4096MB) -> Docker (standard ecosystem)
if [ "$TOTAL_MEM_MB" -lt 4096 ]; then
    RECOMMENDED_ENGINE="podman"
    RECOMMENDED_DESC="Podman (Siêu nhẹ, không tốn daemon RAM nền - Khuyên dùng cho VPS < 4GB RAM)"
    DEFAULT_OPT="1"
else
    RECOMMENDED_ENGINE="docker"
    RECOMMENDED_DESC="Docker Engine (Tương thích phổ biến - Khuyên dùng cho máy >= 4GB RAM)"
    DEFAULT_OPT="2"
fi

DOCKER_INSTALLED="no"
PODMAN_INSTALLED="no"
command -v docker >/dev/null 2>&1 && DOCKER_INSTALLED="yes"
command -v podman >/dev/null 2>&1 && PODMAN_INSTALLED="yes"

if [ "$DOCKER_INSTALLED" = "yes" ] || [ "$PODMAN_INSTALLED" = "yes" ]; then
    echo -e "  🔍 Hiện trạng Container Engine:"
    [ "$DOCKER_INSTALLED" = "yes" ] && echo -e "     - Docker Engine: ${GREEN}Đã cài đặt${NC}"
    [ "$PODMAN_INSTALLED" = "yes" ] && echo -e "     - Podman Engine: ${GREEN}Đã cài đặt${NC}"
fi

CLIENT_IP=""
if [ -n "$SSH_CLIENT" ]; then
    CLIENT_IP=$(echo "$SSH_CLIENT" | awk '{print $1}')
elif [ -n "$SSH_CONNECTION" ]; then
    CLIENT_IP=$(echo "$SSH_CONNECTION" | awk '{print $1}')
fi

if [ -n "$CLIENT_IP" ]; then
    echo -e "  🌐 Địa chỉ IP quản trị của bạn: ${CYAN}${CLIENT_IP}${NC} (sẽ được tự động Whitelist an toàn)"
fi

# ==============================================================================
# [2/6] 30% - Cấu hình Engine, Bảo mật & Tài khoản Quản trị
# ==============================================================================
echo -e "\n${CYAN}${BOLD}[2/6] 30% ⚙️ Cấu hình Engine, Bảo mật & Tài khoản Quản trị...${NC}"

# 2.1 Engine Selection
if [ -z "$ENGINE_CHOICE" ]; then
    echo -e "  ⚙️ Đề xuất cho máy chủ: ${BOLD}${RECOMMENDED_DESC}${NC}"
    echo -e "     ${BOLD}1) Podman${NC}       (Mặc định cho máy < 4GB RAM - Tự động kích hoạt socket API)"
    echo -e "     ${BOLD}2) Docker${NC}       (Mặc định cho máy >= 4GB RAM)"
    echo -e "     ${BOLD}3) Bỏ qua${NC}       (Giữ nguyên engine hiện tại hoặc tự cấu hình sau)"
    
    user_engine_sel=$(prompt_user "  👉 Chọn Engine [1/2/3] (Mặc định: [${DEFAULT_OPT}]): " "$DEFAULT_OPT")
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
echo -e "  👉 Đã chọn Container Engine: ${GREEN}${ENGINE_CHOICE}${NC}"

# 2.2 Fail2ban Selection
if [ -z "$WITH_FAIL2BAN" ]; then
    echo -e "\n  🛡️ [Bảo Vệ Fail2ban] Bạn có muốn tự động cài đặt & cấu hình Fail2ban?"
    echo -e "     - Tự động chặn IP dò quét mật khẩu SSH cổng 22 và Web server."
    echo -e "     - Tự động whitelist IP của bạn (${CLIENT_IP:-'127.0.0.1'})."
    user_f2b_choice=$(prompt_user "  👉 Cài đặt Fail2ban? [Y/n] (Mặc định: Y): " "Y")
    if [[ "$user_f2b_choice" =~ ^[Yy]$ || -z "$user_f2b_choice" ]]; then
        WITH_FAIL2BAN="yes"
    else
        WITH_FAIL2BAN="no"
    fi
fi
echo -e "  👉 Cài đặt Fail2ban: ${GREEN}${WITH_FAIL2BAN}${NC}"

# 2.3 Admin Account Setup
echo -e "\n  👤 [Tài Khoản Quản Trị DockPulse]"
if [ -z "$ADMIN_USER" ]; then
    ADMIN_USER=$(prompt_user "  👉 Nhập Tên đăng nhập quản trị (Mặc định: admin): " "admin")
fi
[ -z "$ADMIN_USER" ] && ADMIN_USER="admin"

if [ -z "$ADMIN_PASS" ]; then
    # Generate a random strong password if none provided or user skips
    RANDOM_PASS=$(tr -dc 'A-Za-z0-9!@#%' </dev/urandom 2>/dev/null | head -c 12 || echo "DockPulse@$(date +%s)")
    if [ "$NON_INTERACTIVE" = "yes" ] || [ ! -r /dev/tty ]; then
        ADMIN_PASS="$RANDOM_PASS"
    else
        user_entered_pass=$(prompt_user "  👉 Nhập Mật khẩu quản trị (Enter để tự sinh mật khẩu an toàn ngẫu nhiên): " "")
        if [ -n "$user_entered_pass" ]; then
            ADMIN_PASS="$user_entered_pass"
        else
            ADMIN_PASS="$RANDOM_PASS"
        fi
    fi
fi
echo -e "  👉 Tài khoản quản trị: ${GREEN}${ADMIN_USER}${NC}"

# ==============================================================================
# [3/6] 50% - Cập nhật hệ thống & Cài đặt gói phụ thuộc
# ==============================================================================
echo -e "\n${CYAN}${BOLD}[3/6] 50% ⏳ Cập nhật hệ thống & Cài đặt gói phụ thuộc...${NC}"

install_apt_deps() {
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq curl tar gzip git systemd rsyslog ca-certificates || true
    if [ "$WITH_FAIL2BAN" = "yes" ]; then
        apt-get install -y -qq fail2ban || true
    fi
    if ! command -v go >/dev/null 2>&1; then
        apt-get install -y -qq golang-go || true
    fi
}

install_dnf_deps() {
    dnf install -y -q curl tar gzip git systemd ca-certificates || true
    if [ "$WITH_FAIL2BAN" = "yes" ]; then
        dnf install -y -q epel-release || true
        dnf install -y -q fail2ban || true
    fi
    if ! command -v go >/dev/null 2>&1; then
        dnf install -y -q golang || true
    fi
}

install_yum_deps() {
    yum install -y -q curl tar gzip git systemd ca-certificates || true
    if [ "$WITH_FAIL2BAN" = "yes" ]; then
        yum install -y -q epel-release || true
        yum install -y -q fail2ban || true
    fi
    if ! command -v go >/dev/null 2>&1; then
        yum install -y -q golang || true
    fi
}

if command -v apt-get >/dev/null 2>&1; then
    run_with_spinner "Cập nhật APT & Cài đặt gói nền tảng" install_apt_deps
elif command -v dnf >/dev/null 2>&1; then
    run_with_spinner "Cập nhật DNF & Cài đặt gói nền tảng" install_dnf_deps
elif command -v yum >/dev/null 2>&1; then
    run_with_spinner "Cập nhật YUM & Cài đặt gói nền tảng" install_yum_deps
fi

# Fallback check for Go: if still not available, download official minimal Go binary archive
if ! command -v go >/dev/null 2>&1; then
    download_go_official() {
        ARCH="$(uname -m)"
        GO_ARCH="amd64"
        [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ] && GO_ARCH="arm64"
        GO_TMP="/tmp/go_install_$$"
        mkdir -p "$GO_TMP"
        GO_TAR="go1.22.6.linux-${GO_ARCH}.tar.gz"
        curl -fsSL "https://go.dev/dl/${GO_TAR}" -o "$GO_TMP/${GO_TAR}"
        tar -C /usr/local -xzf "$GO_TMP/${GO_TAR}"
        export PATH=$PATH:/usr/local/go/bin
        ln -sf /usr/local/go/bin/go /usr/bin/go 2>/dev/null || true
        rm -rf "$GO_TMP"
    }
    run_with_spinner "Tải và cài đặt Golang chính thức (go1.22)" download_go_official
fi

# ==============================================================================
# [4/6] 70% - Cấu hình Container Engine & Bảo mật Fail2ban
# ==============================================================================
echo -e "\n${CYAN}${BOLD}[4/6] 70% 🐳 Cấu hình Container Engine & Tường lửa bảo vệ...${NC}"

if [ "$ENGINE_CHOICE" = "podman" ]; then
    setup_podman() {
        if ! command -v podman >/dev/null 2>&1; then
            if command -v apt-get >/dev/null 2>&1; then
                apt-get install -y -qq podman
            elif command -v dnf >/dev/null 2>&1; then
                dnf install -y -q podman
            elif command -v yum >/dev/null 2>&1; then
                yum install -y -q podman
            fi
        fi

        systemctl daemon-reload
        systemctl enable podman.socket
        systemctl restart podman.socket

        mkdir -p /run/podman /var/run
        if [ ! -e /var/run/docker.sock ] || [ -L /var/run/docker.sock ]; then
            ln -sf /run/podman/podman.sock /var/run/docker.sock
        fi

        mkdir -p /etc/tmpfiles.d
        echo "L+ /var/run/docker.sock - - - - /run/podman/podman.sock" > /etc/tmpfiles.d/podman-docker-socket.conf
        systemd-tmpfiles --create /etc/tmpfiles.d/podman-docker-socket.conf || true
    }
    run_with_spinner "Cấu hình dịch vụ Podman Socket (/run/podman/podman.sock)" setup_podman

elif [ "$ENGINE_CHOICE" = "docker" ]; then
    setup_docker() {
        if ! command -v docker >/dev/null 2>&1; then
            if ! curl -fsSL https://get.docker.com | sh; then
                if command -v apt-get >/dev/null 2>&1; then
                    apt-get install -y -qq docker.io
                elif command -v dnf >/dev/null 2>&1; then
                    dnf install -y -q docker
                elif command -v yum >/dev/null 2>&1; then
                    yum install -y -q docker
                fi
            fi
        fi

        systemctl daemon-reload
        systemctl enable docker
        systemctl restart docker
    }
    run_with_spinner "Cấu hình dịch vụ Docker Engine (/var/run/docker.sock)" setup_docker
fi

# Configure Fail2ban if requested
if [ "$WITH_FAIL2BAN" = "yes" ] && command -v fail2ban-client >/dev/null 2>&1; then
    setup_fail2ban() {
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

        systemctl daemon-reload
        systemctl enable fail2ban
        systemctl restart fail2ban
    }
    run_with_spinner "Cấu hình Fail2ban bảo vệ SSH & Web (Jail rules)" setup_fail2ban
fi

# ==============================================================================
# [5/6] 85% - Tải & Biên dịch ứng dụng DockPulse
# ==============================================================================
echo -e "\n${CYAN}${BOLD}[5/6] 85% ⚙️ Biên dịch & Triển khai DockPulse Hub...${NC}"
mkdir -p "$INSTALL_DIR"

CURRENT_DIR="$(pwd)"
if [ -n "${BASH_SOURCE[0]}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
fi

deploy_dockpulse() {
    if [ -f "$CURRENT_DIR/dockpulse" ] && [ -d "$CURRENT_DIR/web" ]; then
        cp -r "$CURRENT_DIR/dockpulse" "$INSTALL_DIR/"
        cp -r "$CURRENT_DIR/web" "$INSTALL_DIR/"
        [ -f "$CURRENT_DIR/config.json" ] && cp "$CURRENT_DIR/config.json" "$INSTALL_DIR/"
        [ -f "$CURRENT_DIR/.env" ] && cp "$CURRENT_DIR/.env" "$INSTALL_DIR/"
        [ -f "$CURRENT_DIR/.env.example" ] && [ ! -f "$INSTALL_DIR/.env" ] && cp "$CURRENT_DIR/.env.example" "$INSTALL_DIR/.env"
    else
        local BUILD_TMP=$(mktemp -d /tmp/dockpulse_src_XXXXXX)
        local TAR_URL="https://github.com/tientaisv/hostpanel/archive/refs/heads/main.tar.gz"
        
        curl -fsSL "$TAR_URL" | tar -xz -C "$BUILD_TMP" --strip-components=1
        cd "$BUILD_TMP"
        if ! command -v go >/dev/null 2>&1; then
            export PATH=$PATH:/usr/local/go/bin
        fi
        go build -o dockpulse main.go
        cp "$BUILD_TMP/dockpulse" "$INSTALL_DIR/"
        cp -r "$BUILD_TMP/web" "$INSTALL_DIR/"
        [ -f "$BUILD_TMP/config.json" ] && cp "$BUILD_TMP/config.json" "$INSTALL_DIR/"
        [ -f "$BUILD_TMP/.env.example" ] && [ ! -f "$INSTALL_DIR/.env" ] && cp "$BUILD_TMP/.env.example" "$INSTALL_DIR/.env"
        cd "$CURRENT_DIR"
        rm -rf "$BUILD_TMP"
    fi

    # Write configured credentials to .env
    cat << EOF > "$INSTALL_DIR/.env"
ADMIN_USERNAME=$ADMIN_USER
ADMIN_PASSWORD=$ADMIN_PASS
PORT=$PORT
EOF

    chmod +x "$INSTALL_DIR/dockpulse"
}

run_with_spinner "Triển khai ứng dụng & Lưu cấu hình tài khoản" deploy_dockpulse

# ==============================================================================
# [6/6] 100% - Kích hoạt dịch vụ hệ thống & Hoàn tất
# ==============================================================================
echo -e "\n${CYAN}${BOLD}[6/6] 100% 🚀 Kích hoạt dịch vụ hệ thống & Hoàn tất cài đặt...${NC}"

activate_service() {
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
}

run_with_spinner "Kích hoạt dockpulse.service qua systemd" activate_service

# 7. Verification & Information Summary
PUBLIC_IP=$(curl -s -m 3 ifconfig.me 2>/dev/null || curl -s -m 3 api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')

echo -e "\n${GREEN}${BOLD}====================================================================${NC}"
echo -e "${GREEN}${BOLD}           🎉 CHÚC MỪNG! DOCKPULSE ĐÃ ĐƯỢC CÀI ĐẶT THÀNH CÔNG!     ${NC}"
echo -e "${GREEN}${BOLD}====================================================================${NC}"
echo -e "  🌐 ${BOLD}Truy cập Web UI:${NC}    ${CYAN}${BOLD}http://${PUBLIC_IP}:${PORT}${NC}"
echo -e "  👤 ${BOLD}Tài khoản đăng nhập:${NC} ${GREEN}${BOLD}${ADMIN_USER}${NC}"
echo -e "  🔑 ${BOLD}Mật khẩu đăng nhập:${NC}  ${YELLOW}${BOLD}${ADMIN_PASS}${NC}"
echo -e "  --------------------------------------------------------------------"
echo -e "  📂 ${BOLD}Thư mục cài đặt:${NC}    ${INSTALL_DIR}"
echo -e "  ⚙️  ${BOLD}Lệnh kiểm tra:${NC}      systemctl status dockpulse"
if [ "$ENGINE_CHOICE" = "podman" ]; then
    echo -e "  🦭 ${BOLD}Container Engine:${NC}   ${PURPLE}Podman (Socket: /run/podman/podman.sock)${NC}"
elif [ "$ENGINE_CHOICE" = "docker" ]; then
    echo -e "  🐳 ${BOLD}Container Engine:${NC}   ${BLUE}Docker Engine (Socket: /var/run/docker.sock)${NC}"
fi
if [ "$WITH_FAIL2BAN" = "yes" ]; then
    echo -e "  🛡️  ${BOLD}Bảo vệ an ninh:${NC}    ${GREEN}Fail2ban Đang Hoạt Động (SSH & Nginx)${NC}"
fi
echo -e "===================================================================="
echo -e "${YELLOW}👉 LƯU Ý BẢO MẬT:${NC}"
echo -e "  - Vui lòng lưu lại thông tin tài khoản và mật khẩu ở trên."
echo -e "  - Bạn có thể đổi lại mật khẩu bất kỳ lúc nào trực tiếp trên Web UI (nút '🔑 Đổi Mật Khẩu' ở góc trái thanh menu)."
echo -e "${GREEN}✨ Cảm ơn bạn đã sử dụng DockPulse! Chúc bạn có trải nghiệm tuyệt vời!${NC}\n"
