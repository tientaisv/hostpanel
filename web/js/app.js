// Navigation Router & Global App State
document.addEventListener("DOMContentLoaded", () => {
  const navItems = document.querySelectorAll(".nav-item");
  const sections = document.querySelectorAll(".content-section");
  const titleEl = document.getElementById("page-title");
  const subtitleEl = document.getElementById("page-subtitle");

  const titlesMap = {
    dashboard: "Dashboard Hiệu Suất Host",
    "server-info": "Thông Tin Phần Cứng & Hệ Thống Server",
    containers: "Quản Lý Containers",
    compose: "Docker Compose Stacks",
    images: "Quản Lý Docker Images",
    volumes: "Quản Lý Docker Volumes",
    "host-activity": "Hoạt Động Host & Top Tiến Trình (Process Manager)",
    security: "Trung Tâm Bảo Mật System & Cảnh Báo Tấn Công",
    "file-manager": "Trình Quản Lý Tệp Host (Web File Manager)",
    "network-ports": "Quản Lý Cổng Listening & Mạng Docker",
    "host-terminal-tab": "Màn Hình Terminal Server Host Trực Tiếp",
    "ai-assistant": "Trợ Lý AI Phân Tích & Khắc Phục Sự Cố"
  };

  const subtitlesMap = {
    dashboard: "Theo dõi tài nguyên host và container theo thời gian thực.",
    "server-info": "Xem cấu hình phần cứng, hệ điều hành, bộ nhớ và mạng.",
    containers: "Tìm kiếm, theo dõi và thao tác an toàn với từng container.",
    compose: "Theo dõi tài nguyên và trạng thái của từng stack dịch vụ.",
    images: "Quản lý image, dung lượng và các image không còn sử dụng.",
    volumes: "Kiểm tra nơi lưu dữ liệu và dọn volume không sử dụng.",
    "host-activity": "Ưu tiên các tiến trình và tài nguyên đang có ảnh hưởng lớn nhất.",
    security: "Rà soát nguy cơ, Fail2ban và chính sách tường lửa trên host.",
    "file-manager": "Duyệt, quản lý tệp và tìm các thư mục đang chiếm nhiều dung lượng.",
    "network-ports": "Quan sát cổng đang lắng nghe và mạng của container engine.",
    "host-terminal-tab": "Thao tác trực tiếp với host qua phiên terminal trong trình duyệt.",
    "ai-assistant": "Nhận phân tích tình trạng hệ thống và gợi ý xử lý sự cố."
  };

  function activateSection(item) {
    const target = item.getAttribute("data-target");
    if (!target) return;

    navItems.forEach(n => {
      n.classList.remove("active");
      n.removeAttribute("aria-current");
    });
    sections.forEach(s => s.classList.remove("active"));

    item.classList.add("active");
    item.setAttribute("aria-current", "page");
    const sec = document.getElementById(`sec-${target}`);
    if (sec) sec.classList.add("active");

    if (titlesMap[target]) titleEl.textContent = titlesMap[target];
    if (subtitleEl && subtitlesMap[target]) subtitleEl.textContent = subtitlesMap[target];

    // Trigger load for active section
    if (target === "dashboard" && typeof loadWarmupStatus === "function") loadWarmupStatus();
    if (target === "server-info") loadFullServerInfo();
    if (target === "containers") loadContainers();
    if (target === "compose") loadComposeStacks();
    if (target === "images") loadImages();
    if (target === "volumes") loadVolumes();
    if (target === "host-activity") loadHostProcesses();
    if (target === "security") {
      loadSecurityAudit();
      if (typeof loadScannerStatus === "function") loadScannerStatus();
    }
    if (target === "file-manager") loadFiles();
    if (target === "network-ports") loadNetworkPorts();
    if (target === "host-terminal-tab") initHostTabTerminal();
  }

  navItems.forEach(item => {
    item.setAttribute("role", "button");
    item.setAttribute("tabindex", "0");
    if (item.classList.contains("active")) item.setAttribute("aria-current", "page");

    item.addEventListener("click", () => activateSection(item));
    item.addEventListener("keydown", event => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        activateSection(item);
      }
    });
  });

  // Initial load
  initMonitoring();
  loadContainers();
});

function openModal(id) {
  const el = document.getElementById(id);
  if (el) el.classList.add("active");
}

function closeModal(id) {
  const el = document.getElementById(id);
  if (el) el.classList.remove("active");
}

function formatBytes(bytes, decimals = 2) {
  if (bytes === 0) return '0 Bytes';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

function escapeHtml(str) {
  if (str === null || str === undefined) return '';
  return String(str).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

function escapeJsString(str) {
  if (str === null || str === undefined) return '';
  return String(str).replace(/\\/g, '\\\\').replace(/'/g, "\\'");
}
