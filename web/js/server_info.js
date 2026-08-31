// Server Information Page Logic

async function loadFullServerInfo() {
  const btnRefresh = document.getElementById("btn-refresh-server-info");
  if (btnRefresh) {
    btnRefresh.textContent = "⏳ Đang tải...";
    btnRefresh.disabled = true;
  }

  try {
    const res = await fetch("/api/system/full-info");
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json();
    renderFullServerInfo(data);
  } catch (err) {
    console.error("Failed to load server info:", err);
  } finally {
    if (btnRefresh) {
      btnRefresh.textContent = "🔄 Làm Mới";
      btnRefresh.disabled = false;
    }
  }
}

let lastServerInfoData = null;

function renderFullServerInfo(info) {
  if (!info) return;
  lastServerInfoData = info;

  // 1. Hero & Header Overview
  setElText("si-hostname", info.hostname || "--");
  setElText("si-fqdn", `FQDN: ${info.fqdn || info.hostname || '--'}`);
  setElText("si-os-badge", info.os_pretty_name || "Linux");
  setElText("si-os-name", info.os_pretty_name || "Linux");
  setElText("si-kernel-rel", `Kernel: ${info.kernel_release || '--'} (${info.kernel_arch || 'x86_64'})`);
  setElText("si-kernel-version", info.kernel_release || "Linux");
  setElText("si-virt", info.virtualization || "Standard Host");
  setElText("si-product-model", `Model: ${info.product_model || '--'}`);
  setElText("si-uptime", info.uptime_formatted || "--");
  setElText("si-boottime", `Khởi động lúc: ${info.boot_time || '--'}`);

  // 2. CPU Hardware Details
  const cpu = info.cpu || {};
  setElText("si-cpu-arch", cpu.architecture || info.kernel_arch || "x86_64");
  setElText("si-cpu-model", cpu.model_name || "--");
  setElText("si-cpu-phys-cores", cpu.physical_cores || 1);
  setElText("si-cpu-log-threads", cpu.logical_threads || 1);
  
  let freqStr = `${cpu.cur_freq_mhz ? cpu.cur_freq_mhz.toFixed(0) : '--'} MHz`;
  if (cpu.min_freq_mhz > 0 || cpu.max_freq_mhz > 0) {
    freqStr += ` (Min: ${cpu.min_freq_mhz ? cpu.min_freq_mhz.toFixed(0) : '--'} MHz / Max: ${cpu.max_freq_mhz ? cpu.max_freq_mhz.toFixed(0) : '--'} MHz)`;
  }
  setElText("si-cpu-freq", freqStr);

  if (cpu.temperature_c > 0) {
    setElText("si-cpu-temp", `🔥 ${cpu.temperature_c.toFixed(1)} °C`);
  } else {
    setElText("si-cpu-temp", "N/A (VM/Virtual)");
  }

  if (cpu.caches && cpu.caches.length > 0) {
    const cacheStr = cpu.caches.map(c => `${c.level} (${c.type}): ${c.size}`).join(" | ");
    setElText("si-cpu-caches", cacheStr);
  } else {
    setElText("si-cpu-caches", "N/A");
  }

  setElText("si-cpu-governor", cpu.governor || "default");

  const flagsEl = document.getElementById("si-cpu-flags");
  if (flagsEl) {
    if (cpu.key_flags && cpu.key_flags.length > 0) {
      flagsEl.innerHTML = cpu.key_flags.map(f => `<span class="badge badge-secondary" style="font-size: 0.75rem; font-family: monospace; background: rgba(56, 189, 248, 0.1); border-color: rgba(56, 189, 248, 0.2); color: #38bdf8;">${escapeHtml(f)}</span>`).join("");
    } else {
      flagsEl.innerHTML = `<span style="color: var(--text-muted); font-size: 0.8rem;">Standard x86/ARM Features</span>`;
    }
  }

  // 3. System Hardware & Runtimes
  setElText("si-hw-vendor", info.product_vendor || "--");
  setElText("si-hw-product", info.product_model || "--");
  setElText("si-bios-info", `${info.bios_version || 'N/A'} ${info.bios_date ? '(' + info.bios_date + ')' : ''}`);
  setElText("si-total-procs", `${info.total_processes || 0} tiến trình (Đang đăng nhập: ${info.logged_users_count || 0} người dùng)`);
  setElText("si-file-nr", info.file_descriptors || "N/A");
  
  let runtimesStr = `Go ${info.go_version || '--'}`;
  if (info.systemd_version) runtimesStr += ` | ${info.systemd_version}`;
  if (info.docker_version) runtimesStr += ` | ${info.docker_version.split(',')[0]}`;
  if (info.podman_version) runtimesStr += ` | Podman ${info.podman_version.split(' ')[0]}`;
  setElText("si-runtimes", runtimesStr);
  setElText("si-systime", `${info.current_time || '--'} (${info.timezone || '--'})`);

  // 4. Memory & Swap Breakdown
  const mem = info.memory || {};
  const ramUsed = mem.used_mb || 0;
  const ramTotal = mem.total_mb || 1;
  const ramAvail = mem.available_mb || 0;
  const ramPct = mem.percent ? mem.percent.toFixed(1) : ((ramUsed / ramTotal) * 100).toFixed(1);

  setElText("si-ram-summary", `${formatMBHelper(ramUsed)} / ${formatMBHelper(ramTotal)}`);
  setElText("si-ram-pct", `${ramPct}%`);
  setElText("si-ram-used", formatMBHelper(ramUsed));
  setElText("si-ram-avail", formatMBHelper(ramAvail));
  setElText("si-ram-cached", formatMBHelper(mem.cached_mb || 0));
  setElText("si-ram-buffers", formatMBHelper(mem.buffers_mb || 0));

  const ramBar = document.getElementById("si-ram-bar");
  if (ramBar) {
    ramBar.style.width = `${Math.min(100, Math.max(0, ramPct))}%`;
  }

  const swapUsed = mem.swap_used_mb || 0;
  const swapTotal = mem.swap_total_mb || 0;
  const swapPct = swapTotal > 0 ? ((swapUsed / swapTotal) * 100).toFixed(1) : "0.0";
  setElText("si-swap-summary", `${formatMBHelper(swapUsed)} / ${formatMBHelper(swapTotal)}`);
  setElText("si-swap-pct", `${swapPct}%`);
  setElText("si-swap-used", formatMBHelper(swapUsed));
  setElText("si-swap-free", formatMBHelper(mem.swap_free_mb || 0));
  setElText("si-swappiness-badge", `Swappiness: ${mem.swappiness !== undefined ? mem.swappiness : 60}`);
  
  if (mem.hugepages_total > 0) {
    setElText("si-hugepages", `${mem.hugepages_total} pages (${mem.hugepage_size_kb || 2048} KB)`);
  } else {
    setElText("si-hugepages", "None / Disabled");
  }

  const swapBar = document.getElementById("si-swap-bar");
  if (swapBar) {
    swapBar.style.width = `${Math.min(100, Math.max(0, swapPct))}%`;
  }

  // 5. Mount Points Table
  renderMountsTable(info.mounts);

  // 6. Network Interfaces Table
  renderInterfacesTable(info.interfaces, info.public_ip, info.default_gateway, info.dns_servers);
}

function renderMountsTable(mounts) {
  const tbody = document.getElementById("tbody-server-mounts");
  if (!tbody) return;

  if (!mounts || mounts.length === 0) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align: center; color: var(--text-muted);">Không tìm thấy thông tin phân vùng nào.</td></tr>`;
    return;
  }

  tbody.innerHTML = mounts.map(m => {
    const pct = m.percent ? m.percent.toFixed(1) : "0.0";
    let barColor = "var(--accent-blue)";
    if (m.percent > 85) barColor = "var(--accent-red)";
    else if (m.percent > 70) barColor = "var(--accent-amber)";

    return `
      <tr>
        <td style="font-weight: 600; font-family: monospace; color: var(--text-main);">${escapeHtml(m.filesystem)}</td>
        <td style="font-weight: 500; color: #38bdf8;">${escapeHtml(m.mount_point)}</td>
        <td><span class="badge badge-secondary" style="font-family: monospace;">${escapeHtml(m.fstype)}</span></td>
        <td style="font-weight: 600;">${m.total_gb ? m.total_gb.toFixed(1) + ' GB' : '--'}</td>
        <td style="color: var(--text-muted);">${m.used_gb ? m.used_gb.toFixed(1) + ' GB' : '--'}</td>
        <td style="color: #4ade80; font-weight: 600;">${m.available_gb ? m.available_gb.toFixed(1) + ' GB' : '--'}</td>
        <td>
          <div style="display: flex; align-items: center; gap: 8px;">
            <div class="progress-bar" style="flex: 1; height: 6px; background: rgba(255,255,255,0.06);">
              <div class="progress-fill" style="width: ${Math.min(100, Math.max(0, m.percent))}%; background: ${barColor};"></div>
            </div>
            <span style="font-size: 0.8rem; font-weight: 600; width: 42px; text-align: right;">${pct}%</span>
          </div>
        </td>
      </tr>
    `;
  }).join("");
}

function renderInterfacesTable(interfaces, publicIP, defaultGW, dnsServers) {
  setElText("si-net-pubip", publicIP || "N/A");
  setElText("si-net-gw", defaultGW || "N/A");

  const tbody = document.getElementById("tbody-server-ifaces");
  if (!tbody) return;

  if (!interfaces || interfaces.length === 0) {
    tbody.innerHTML = `<tr><td colspan="7" style="text-align: center; color: var(--text-muted);">Không tìm thấy card mạng nào.</td></tr>`;
    return;
  }

  tbody.innerHTML = interfaces.map(iface => {
    const isUp = iface.state === "UP";
    const stateBadge = isUp 
      ? `<span class="badge badge-primary" style="background: rgba(74, 222, 128, 0.15); color: #4ade80; border-color: rgba(74, 222, 128, 0.3);">🟢 UP</span>`
      : `<span class="badge badge-secondary" style="color: var(--text-muted);">⚪ DOWN</span>`;

    const ips = [];
    if (iface.ipv4 && iface.ipv4.length > 0) {
      iface.ipv4.forEach(ip => ips.push(`<div style="color: #38bdf8; font-weight: 500;">${escapeHtml(ip)}</div>`));
    }
    if (iface.ipv6 && iface.ipv6.length > 0) {
      iface.ipv6.forEach(ip => ips.push(`<div style="font-size: 0.75rem; color: #a78bfa; font-family: monospace;">${escapeHtml(ip)}</div>`));
    }
    const ipStr = ips.length > 0 ? ips.join("") : '<span style="color: var(--text-muted);">-</span>';

    const rxFormatted = formatBytesHelper(iface.rx_bytes);
    const txFormatted = formatBytesHelper(iface.tx_bytes);

    return `
      <tr>
        <td style="font-weight: 700; color: var(--text-main);">
          ${escapeHtml(iface.name)}
          ${iface.is_virtual ? '<span style="font-size: 0.7rem; color: var(--text-muted); margin-left: 4px;">(Virtual)</span>' : ''}
        </td>
        <td>${stateBadge}</td>
        <td style="font-family: monospace; font-size: 0.82rem; color: var(--text-muted);">${escapeHtml(iface.mac || '-')}</td>
        <td>${ipStr}</td>
        <td style="font-size: 0.85rem;">${escapeHtml(iface.speed || '--')} <span style="color: var(--text-muted); font-size: 0.75rem;">(MTU: ${iface.mtu})</span></td>
        <td style="color: #38bdf8; font-weight: 600;">📥 ${rxFormatted}</td>
        <td style="color: #c084fc; font-weight: 600;">📤 ${txFormatted}</td>
      </tr>
    `;
  }).join("");
}

function setElText(id, text) {
  const el = document.getElementById(id);
  if (el) el.textContent = text;
}

function formatMBHelper(mb) {
  if (mb === undefined || mb === null) return "--";
  if (mb >= 1024) {
    return (mb / 1024).toFixed(2) + " GB";
  }
  return mb.toFixed(0) + " MB";
}

function formatBytesHelper(bytes) {
  if (!bytes || bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

function copyServerInfoMarkdown() {
  if (!lastServerInfoData) {
    alert("Dữ liệu thông tin server chưa sẵn sàng, vui lòng đợi vài giây!");
    return;
  }
  const d = lastServerInfoData;
  const cpu = d.cpu || {};
  const mem = d.memory || {};

  const md = `# 🖥️ BÁO CÁO CẤU HÌNH SERVER (${d.hostname})
- **Thời gian quét:** ${d.current_time} (${d.timezone})
- **Hệ điều hành:** ${d.os_pretty_name} (${d.kernel_arch})
- **Linux Kernel:** ${d.kernel_release}
- **Loại ảo hóa:** ${d.virtualization}
- **Phần cứng/Model:** ${d.product_vendor} - ${d.product_model}
- **Thời gian Uptime:** ${d.uptime_formatted} (Boot: ${d.boot_time})
- **Địa chỉ IP Public:** ${d.public_ip}
- **Default Gateway:** ${d.default_gateway}

---
### ⚡ VI XỬ LÝ (CPU)
- **Model:** ${cpu.model_name}
- **Cores / Threads:** ${cpu.physical_cores} Cores vật lý / ${cpu.logical_threads} vCPU Threads
- **Tần số xung nhịp:** ${cpu.cur_freq_mhz ? cpu.cur_freq_mhz.toFixed(0) : '--'} MHz
- **Nhiệt độ:** ${cpu.temperature_c ? cpu.temperature_c.toFixed(1) + ' °C' : 'N/A'}
- **CPU Flags:** ${cpu.key_flags ? cpu.key_flags.join(", ") : 'N/A'}

---
### 🧠 BỘ NHỚ RAM & SWAP
- **RAM Total / Used:** ${formatMBHelper(mem.used_mb)} / ${formatMBHelper(mem.total_mb)} (${mem.percent ? mem.percent.toFixed(1) : '0'}%)
- **RAM Khả dụng:** ${formatMBHelper(mem.available_mb)} (Cached: ${formatMBHelper(mem.cached_mb)})
- **Swap:** ${formatMBHelper(mem.swap_used_mb)} / ${formatMBHelper(mem.swap_total_mb)} (Swappiness: ${mem.swappiness})

---
### 💾 PHÂN VÙNG Ổ CỨNG (MOUNTS)
${(d.mounts || []).map(m => `- \`${m.mount_point}\` (${m.filesystem}, ${m.fstype}): ${m.used_gb.toFixed(1)}GB / ${m.total_gb.toFixed(1)}GB (${m.percent.toFixed(1)}%)`).join("\n")}
`;

  navigator.clipboard.writeText(md).then(() => {
    const btn = document.getElementById("btn-copy-server-info");
    if (btn) {
      const orig = btn.textContent;
      btn.textContent = "✅ Đã Sao Chép!";
      setTimeout(() => { btn.textContent = orig; }, 2500);
    }
  }).catch(() => {
    alert("Không thể tự động sao chép. Vui lòng cho phép quyền truy cập Clipboard trên trình duyệt.");
  });
}
