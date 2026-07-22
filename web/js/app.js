// Navigation Router & Global App State
document.addEventListener("DOMContentLoaded", () => {
  const navItems = document.querySelectorAll(".nav-item");
  const sections = document.querySelectorAll(".content-section");
  const titleEl = document.getElementById("page-title");

  const titlesMap = {
    dashboard: "Dashboard Hiệu Suất Host",
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

  navItems.forEach(item => {
    item.addEventListener("click", () => {
      const target = item.getAttribute("data-target");

      navItems.forEach(n => n.classList.remove("active"));
      sections.forEach(s => s.classList.remove("active"));

      item.classList.add("active");
      const sec = document.getElementById(`sec-${target}`);
      if (sec) sec.classList.add("active");

      if (titlesMap[target]) {
        titleEl.textContent = titlesMap[target];
      }

      // Trigger load for active section
      if (target === "containers") loadContainers();
      if (target === "compose") loadComposeStacks();
      if (target === "images") loadImages();
      if (target === "volumes") loadVolumes();
      if (target === "host-activity") loadHostProcesses();
      if (target === "security") loadSecurityAudit();
      if (target === "file-manager") loadFiles();
      if (target === "network-ports") loadNetworkPorts();
      if (target === "host-terminal-tab") initHostTabTerminal();
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
