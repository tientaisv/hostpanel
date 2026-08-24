let hostChartInstance = null;
const maxDataPoints = 30;
let currentChartMode = 'live';

function formatHumanBytes(bytes, decimals = 1) {
  if (!bytes || bytes === 0 || isNaN(bytes)) return '0 B';
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
  const val = bytes / Math.pow(k, i);
  return parseFloat(val.toFixed(dm)) + ' ' + sizes[i];
}

function initMonitoring() {
  const ctx = document.getElementById("hostChart")?.getContext("2d");
  if (ctx) {
    hostChartInstance = new Chart(ctx, {
      type: "line",
      data: {
        labels: [],
        datasets: [
          {
            label: "Host CPU (%)",
            borderColor: "#38bdf8",
            backgroundColor: "rgba(56, 189, 248, 0.1)",
            fill: false,
            tension: 0.3,
            pointRadius: 0,
            pointHoverRadius: 6,
            pointHitRadius: 10,
            borderWidth: 2,
            data: []
          },
          {
            label: "Host RAM (%)",
            borderColor: "#6366f1",
            backgroundColor: "rgba(99, 102, 241, 0.1)",
            fill: false,
            tension: 0.3,
            pointRadius: 0,
            pointHoverRadius: 6,
            pointHitRadius: 10,
            borderWidth: 2,
            data: []
          },
          {
            label: "Docker Total CPU (%)",
            borderColor: "#34d399",
            backgroundColor: "rgba(52, 211, 153, 0.1)",
            borderDash: [4, 4],
            fill: false,
            tension: 0.3,
            pointRadius: 0,
            pointHoverRadius: 6,
            pointHitRadius: 10,
            borderWidth: 2,
            data: []
          },
          {
            label: "Docker Total RAM (%)",
            borderColor: "#a855f7",
            backgroundColor: "rgba(168, 85, 247, 0.1)",
            borderDash: [4, 4],
            fill: false,
            tension: 0.3,
            pointRadius: 0,
            pointHoverRadius: 6,
            pointHitRadius: 10,
            borderWidth: 2,
            data: []
          }
        ]
      },
      options: {
        responsive: true,
        elements: {
          point: {
            radius: 0,
            hoverRadius: 6,
            hitRadius: 10
          },
          line: {
            borderWidth: 2
          }
        },
        scales: {
          x: { grid: { color: "rgba(255,255,255,0.05)" }, ticks: { color: "#94a3b8", maxTicksLimit: 12 } },
          y: { min: 0, max: 100, grid: { color: "rgba(255,255,255,0.05)" }, ticks: { color: "#94a3b8" } }
        },
        plugins: {
          legend: { labels: { color: "#f8fafc" } }
        }
      }
    });
  }

  // Connect WebSocket for host metrics
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsURL = `${protocol}//${window.location.host}/ws/stats`;

  const ws = new WebSocket(wsURL);

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      updateDashboardUI(data);
    } catch (e) {}
  };

  ws.onerror = () => {
    setInterval(async () => {
      try {
        const res = await fetch("/api/host");
        if (res.ok) {
          const data = await res.json();
          updateDashboardUI(data);
        }
      } catch (e) {}
    }, 3000);
  };
}

let currentStatsData = null;

function formatUptime(uptimeSec) {
  if (!uptimeSec) return '--';
  const days = Math.floor(uptimeSec / 86400);
  const hours = Math.floor((uptimeSec % 86400) / 3600);
  const mins = Math.floor((uptimeSec % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h ${mins}m`;
  if (hours > 0) return `${hours}h ${mins}m`;
  return `${mins}m`;
}

function updateDashboardUI(data) {
  if (!data) return;
  currentStatsData = data;

  // Host CPU
  const cpuVal = data.cpu_percent ? data.cpu_percent.toFixed(1) : 0;
  if (document.getElementById("host-cpu")) document.getElementById("host-cpu").textContent = `${cpuVal}%`;
  if (document.getElementById("fill-cpu")) document.getElementById("fill-cpu").style.width = `${Math.min(cpuVal, 100)}%`;

  // Host RAM - Human readable
  const ramUsedBytes = (data.mem_used_mb || 0) * 1024 * 1024;
  const ramTotalBytes = (data.mem_total_mb || 0) * 1024 * 1024;
  const ramVal = data.mem_percent ? data.mem_percent.toFixed(1) : 0;

  if (document.getElementById("host-ram")) {
    document.getElementById("host-ram").textContent = `${formatHumanBytes(ramUsedBytes)} / ${formatHumanBytes(ramTotalBytes)}`;
  }
  if (document.getElementById("fill-ram")) document.getElementById("fill-ram").style.width = `${Math.min(ramVal, 100)}%`;

  // Swap - Human readable
  const swapUsedBytes = (data.swap_used_mb || 0) * 1024 * 1024;
  const swapTotalBytes = (data.swap_total_mb || 0) * 1024 * 1024;
  const swapVal = data.swap_percent ? data.swap_percent.toFixed(1) : 0;

  if (document.getElementById("host-swap")) {
    document.getElementById("host-swap").textContent = `${formatHumanBytes(swapUsedBytes)} / ${formatHumanBytes(swapTotalBytes)}`;
  }
  if (document.getElementById("fill-swap")) document.getElementById("fill-swap").style.width = `${Math.min(swapVal, 100)}%`;

  // Disk Space
  const diskVal = data.disk_percent ? data.disk_percent.toFixed(1) : 0;
  if (document.getElementById("host-disk")) {
    document.getElementById("host-disk").textContent = `${data.disk_used_gb || 0} GB / ${data.disk_total_gb || 0} GB`;
  }
  if (document.getElementById("fill-disk")) document.getElementById("fill-disk").style.width = `${Math.min(diskVal, 100)}%`;

  // Disk I/O & Net I/O
  if (document.getElementById("host-disk-io")) {
    const rSpeedBytes = (data.disk_read_rate_mb || 0) * 1024 * 1024;
    const wSpeedBytes = (data.disk_write_rate_mb || 0) * 1024 * 1024;
    const rAccBytes = (data.disk_read_mb || 0) * 1024 * 1024;
    const wAccBytes = (data.disk_write_mb || 0) * 1024 * 1024;

    document.getElementById("host-disk-io").innerHTML = `
      <div style="font-size: 1.3rem; font-weight: 700; color: #38bdf8;">
        📖 ${formatHumanBytes(rSpeedBytes)}/s | ✍️ ${formatHumanBytes(wSpeedBytes)}/s
      </div>
      <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">
        Tổng tích lũy: 📖 ${formatHumanBytes(rAccBytes)} | ✍️ ${formatHumanBytes(wAccBytes)}
      </div>
    `;
  }
  if (document.getElementById("host-net-io")) {
    const rxSpeedBytes = (data.net_rx_rate_kb || 0) * 1024;
    const txSpeedBytes = (data.net_tx_rate_kb || 0) * 1024;
    const rxAccBytes = (data.net_rx_kb || 0) * 1024;
    const txAccBytes = (data.net_tx_kb || 0) * 1024;

    document.getElementById("host-net-io").innerHTML = `
      <div style="font-size: 1.3rem; font-weight: 700; color: #a855f7;">
        📥 ${formatHumanBytes(rxSpeedBytes)}/s | 📤 ${formatHumanBytes(txSpeedBytes)}/s
      </div>
      <div style="font-size: 0.75rem; color: var(--text-muted); margin-top: 4px;">
        Tổng tích lũy: 📬 ${formatHumanBytes(rxAccBytes)} | 📤 ${formatHumanBytes(txAccBytes)}
      </div>
    `;
  }

  // Containers & Load
  if (document.getElementById("host-ctrs")) document.getElementById("host-ctrs").textContent = data.containers_count || 0;
  if (document.getElementById("host-load")) document.getElementById("host-load").textContent = `Load: ${data.load_1 || 0.00}, ${data.load_5 || 0.00}, ${data.load_15 || 0.00}`;

  // Server Static & Hardware Details
  if (document.getElementById("srv-hostname")) document.getElementById("srv-hostname").textContent = data.hostname || "Linux Host";
  if (document.getElementById("srv-os")) document.getElementById("srv-os").textContent = data.os_info || "Linux";
  if (document.getElementById("srv-kernel")) document.getElementById("srv-kernel").textContent = `Kernel: ${data.kernel_version || '-'}`;
  if (document.getElementById("srv-cpumodel")) document.getElementById("srv-cpumodel").textContent = data.cpu_model || "CPU Processor";
  if (document.getElementById("srv-cores")) document.getElementById("srv-cores").textContent = `${data.cores_count || 1} Cores / Threads`;
  if (document.getElementById("srv-uptime")) document.getElementById("srv-uptime").textContent = formatUptime(data.uptime_sec);

  // CPU Core Performance Grid Breakdown
  if (document.getElementById("cpu-cores-container")) {
    if (data.cpu_core_percents && data.cpu_core_percents.length > 0) {
      const coresHtml = data.cpu_core_percents.map(c => {
        const pctVal = c.percent ? c.percent.toFixed(1) : "0.0";
        let color = "#34d399";
        if (c.percent > 85) color = "#f87171";
        else if (c.percent > 60) color = "#fbbf24";

        return `
          <div style="background: #0d1117; padding: 10px 12px; border-radius: 6px; border: 1px solid var(--border-color);">
            <div style="display: flex; justify-content: space-between; font-size: 0.8rem; margin-bottom: 6px;">
              <span style="font-weight: 600; color: var(--text-main);">Core ${c.core_id}</span>
              <span style="font-weight: 700; color: ${color};">${pctVal}%</span>
            </div>
            <div class="progress-bar" style="height: 6px; background: rgba(255,255,255,0.08);">
              <div class="progress-fill" style="width: ${Math.min(c.percent, 100)}%; background: ${color}; border-radius: 3px;"></div>
            </div>
          </div>
        `;
      }).join('');
      document.getElementById("cpu-cores-container").innerHTML = coresHtml;
    }
  }

  // Dynamic Container Engine (Podman vs Docker) presentation
  const isPodman = data.is_podman !== undefined ? data.is_podman : true;
  const engineName = data.engine_name || (isPodman ? "Podman" : "Docker");
  const engineVersion = data.engine_version ? ` v${data.engine_version}` : "";
  const engineIcon = isPodman ? "🦭" : "🐳";

  if (document.getElementById("engine-badge-text")) {
    document.getElementById("engine-badge-text").textContent = `${engineName}${engineVersion}`;
  }
  if (document.getElementById("engine-badge-icon")) {
    document.getElementById("engine-badge-icon").textContent = engineIcon;
  }
  if (document.getElementById("nav-engine-category")) {
    document.getElementById("nav-engine-category").textContent = `${engineName.toUpperCase()} ENGINE`;
  }
  if (document.getElementById("card-engine-ctrs-title")) {
    document.getElementById("card-engine-ctrs-title").textContent = `${engineName} Containers`;
  }
  if (document.getElementById("card-engine-ctrs-icon")) {
    document.getElementById("card-engine-ctrs-icon").textContent = engineIcon;
  }
  if (document.getElementById("engine-pool-title")) {
    document.getElementById("engine-pool-title").innerHTML = `<span>${engineIcon}</span> Tổng Tài Nguyên ${engineName} Engine Đang Sử Dụng`;
  }

  // Total Container Engine Resource Consumption
  if (document.getElementById("docker-total-running-badge")) {
    document.getElementById("docker-total-running-badge").textContent = `${data.docker_running_count || 0} / ${data.containers_count || 0} Running Containers`;
  }
  const dCpuPct = data.docker_cpu_percent ? data.docker_cpu_percent.toFixed(1) : 0;
  if (document.getElementById("docker-total-cpu")) {
    document.getElementById("docker-total-cpu").textContent = `${dCpuPct}%`;
  }
  if (document.getElementById("fill-docker-cpu")) {
    document.getElementById("fill-docker-cpu").style.width = `${Math.min(dCpuPct, 100)}%`;
  }

  const dMemUsedMB = data.docker_mem_used_mb || 0;
  const dMemUsedBytes = dMemUsedMB * 1024 * 1024;
  const dMemPct = data.docker_mem_percent ? data.docker_mem_percent.toFixed(1) : 0;
  if (document.getElementById("docker-total-ram")) {
    document.getElementById("docker-total-ram").textContent = `${formatHumanBytes(dMemUsedBytes)} (${dMemPct}% Server RAM)`;
  }
  if (document.getElementById("fill-docker-ram")) {
    document.getElementById("fill-docker-ram").style.width = `${Math.min(dMemPct, 100)}%`;
  }

  const dNetRxBytes = (data.docker_net_rx_mb || 0) * 1024 * 1024;
  const dNetTxBytes = (data.docker_net_tx_mb || 0) * 1024 * 1024;
  if (document.getElementById("docker-total-net")) {
    document.getElementById("docker-total-net").textContent = `📥 ${formatHumanBytes(dNetRxBytes)} | 📤 ${formatHumanBytes(dNetTxBytes)}`;
  }

  // Realtime Chart Update (Only in 'live' mode)
  if (hostChartInstance && currentChartMode === 'live') {
    const timeLabel = new Date().toLocaleTimeString();
    hostChartInstance.data.labels.push(timeLabel);
    hostChartInstance.data.datasets[0].data.push(cpuVal);
    hostChartInstance.data.datasets[1].data.push(ramVal);
    hostChartInstance.data.datasets[2].data.push(dCpuPct);
    hostChartInstance.data.datasets[3].data.push(dMemPct);

    if (hostChartInstance.data.labels.length > maxDataPoints) {
      hostChartInstance.data.labels.shift();
      hostChartInstance.data.datasets[0].data.shift();
      hostChartInstance.data.datasets[1].data.shift();
      hostChartInstance.data.datasets[2].data.shift();
      hostChartInstance.data.datasets[3].data.shift();
    }

    hostChartInstance.update();
  }
}

async function setChartMode(mode) {
  currentChartMode = mode;
  document.querySelectorAll(".history-btn").forEach(btn => btn.classList.remove("active"));
  const activeBtn = document.getElementById(`btn-chart-${mode}`);
  if (activeBtn) activeBtn.classList.add("active");

  if (!hostChartInstance) return;

  if (mode === "live") {
    hostChartInstance.data.labels = [];
    hostChartInstance.data.datasets.forEach(ds => ds.data = []);
    hostChartInstance.update();
    return;
  }

  try {
    const res = await fetch(`/api/metrics/history?range=${mode}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const records = await res.json();
    renderHistoricalChart(records);
  } catch (err) {
    console.error("Failed to load metrics history:", err);
  }
}

function renderHistoricalChart(records) {
  if (!hostChartInstance || !records) return;

  // Downsample historical data points if records exceed max limit (e.g. 120)
  let sampledRecords = records;
  const maxPoints = 120;
  if (records.length > maxPoints) {
    const step = Math.ceil(records.length / maxPoints);
    sampledRecords = records.filter((_, index) => index % step === 0);
    if (records.length > 0 && sampledRecords[sampledRecords.length - 1] !== records[records.length - 1]) {
      sampledRecords.push(records[records.length - 1]);
    }
  }

  const labels = [];
  const hostCpuData = [];
  const hostRamData = [];
  const dockerCpuData = [];
  const dockerRamData = [];

  sampledRecords.forEach(rec => {
    const dt = new Date(rec.recorded_at);
    const labelStr = currentChartMode === '24h' ? dt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : `${dt.getMonth()+1}/${dt.getDate()} ${dt.getHours()}:00`;

    labels.push(labelStr);
    hostCpuData.push(rec.host_cpu_percent ? rec.host_cpu_percent.toFixed(1) : 0);

    const hRamPct = (rec.host_ram_total_mb > 0) ? ((rec.host_ram_used_mb / rec.host_ram_total_mb) * 100).toFixed(1) : 0;
    hostRamData.push(hRamPct);

    dockerCpuData.push(rec.docker_cpu_percent ? rec.docker_cpu_percent.toFixed(1) : 0);

    const dRamPct = (rec.host_ram_total_mb > 0) ? ((rec.docker_ram_used_mb / rec.host_ram_total_mb) * 100).toFixed(1) : 0;
    dockerRamData.push(dRamPct);
  });

  hostChartInstance.data.labels = labels;
  hostChartInstance.data.datasets[0].data = hostCpuData;
  hostChartInstance.data.datasets[1].data = hostRamData;
  hostChartInstance.data.datasets[2].data = dockerCpuData;
  hostChartInstance.data.datasets[3].data = dockerRamData;
  hostChartInstance.update();
}

function triggerResetSwapModal() {
  const swapUsedMB = currentStatsData ? (currentStatsData.swap_used_mb || 0) : 0;
  const memTotal = currentStatsData ? (currentStatsData.mem_total_mb || 0) : 0;
  const memUsed = currentStatsData ? (currentStatsData.mem_used_mb || 0) : 0;
  const memFreeMB = Math.max(0, memTotal - memUsed);

  if (document.getElementById("modal-swap-used")) {
    document.getElementById("modal-swap-used").textContent = `${swapUsedMB} MB`;
  }
  if (document.getElementById("modal-ram-avail")) {
    document.getElementById("modal-ram-avail").textContent = `${memFreeMB} MB`;
  }

  const btnConfirm = document.getElementById("btn-confirm-reset-swap");
  const safetyBadge = document.getElementById("modal-swap-safety");
  const statusMsg = document.getElementById("reset-swap-status-msg");

  if (statusMsg) {
    statusMsg.style.display = "none";
    statusMsg.textContent = "";
  }

  if (swapUsedMB === 0) {
    if (safetyBadge) {
      safetyBadge.className = "badge badge-running";
      safetyBadge.textContent = "Swap đang trống (0 MB)";
    }
    if (btnConfirm) btnConfirm.disabled = false;
  } else if (memFreeMB > swapUsedMB) {
    if (safetyBadge) {
      safetyBadge.className = "badge badge-running";
      safetyBadge.textContent = "✅ Đủ RAM để xả Swap";
    }
    if (btnConfirm) btnConfirm.disabled = false;
  } else {
    if (safetyBadge) {
      safetyBadge.className = "badge badge-danger";
      safetyBadge.textContent = "⚠️ RAM không đủ để xả Swap!";
    }
    if (btnConfirm) btnConfirm.disabled = true;
  }

  const modal = document.getElementById("modal-reset-swap");
  if (modal) modal.classList.add("active");
}

async function executeResetSwap() {
  const btnConfirm = document.getElementById("btn-confirm-reset-swap");
  const statusMsg = document.getElementById("reset-swap-status-msg");

  if (btnConfirm) {
    btnConfirm.disabled = true;
    btnConfirm.textContent = "⏳ Đang giải phóng Swap...";
  }

  if (statusMsg) {
    statusMsg.style.display = "block";
    statusMsg.style.background = "rgba(56, 189, 248, 0.15)";
    statusMsg.style.color = "#38bdf8";
    statusMsg.style.border = "1px solid rgba(56, 189, 248, 0.3)";
    statusMsg.textContent = "Đang thực hiện swapoff -a && swapon -a. Vui lòng chờ...";
  }

  try {
    const res = await fetch("/api/system/swap/reset", { method: "POST" });
    const data = await res.json();
    
    if (res.ok) {
      if (statusMsg) {
        statusMsg.style.background = "rgba(52, 211, 153, 0.15)";
        statusMsg.style.color = "#34d399";
        statusMsg.style.border = "1px solid rgba(52, 211, 153, 0.3)";
        statusMsg.textContent = `✅ ${data.message || "Reset Swap thành công!"}`;
      }
      setTimeout(() => {
        const modal = document.getElementById("modal-reset-swap");
        if (modal) modal.classList.remove("active");
      }, 2500);
    } else {
      if (statusMsg) {
        statusMsg.style.background = "rgba(248, 113, 113, 0.15)";
        statusMsg.style.color = "#f87171";
        statusMsg.style.border = "1px solid rgba(248, 113, 113, 0.3)";
        statusMsg.textContent = `❌ ${data.error || "Không thể reset Swap"}`;
      }
    }
  } catch (err) {
    if (statusMsg) {
      statusMsg.style.background = "rgba(248, 113, 113, 0.15)";
      statusMsg.style.color = "#f87171";
      statusMsg.style.border = "1px solid rgba(248, 113, 113, 0.3)";
      statusMsg.textContent = `❌ Lỗi kết nối server: ${err.message}`;
    }
  } finally {
    if (btnConfirm) {
      btnConfirm.disabled = false;
      btnConfirm.textContent = "⚡ Xác Nhận Reset Swap";
    }
  }
}

function triggerPwmConfigModal() {
  const channelInput = document.getElementById("pwm-channel-input");
  const speedInput = document.getElementById("pwm-speed-input");
  const statusMsg = document.getElementById("pwm-status-msg");
  const btnConfirm = document.getElementById("btn-confirm-pwm");

  if (channelInput) channelInput.value = 0;
  if (speedInput) speedInput.value = 255;
  if (statusMsg) {
    statusMsg.style.display = "none";
    statusMsg.textContent = "";
  }
  if (btnConfirm) {
    btnConfirm.disabled = false;
    btnConfirm.innerHTML = '<span>⚡</span> Chạy Lệnh PWM (255 Max)';
  }
  updatePwmCmdPreview();

  const modal = document.getElementById("modal-pwmconfig");
  if (modal) modal.classList.add("active");
}

function updatePwmCmdPreview() {
  const c = document.getElementById("pwm-channel-input")?.value || 0;
  const s = document.getElementById("pwm-speed-input")?.value || 255;
  const preview = document.getElementById("pwm-preview-cmd");
  if (preview) {
    preview.textContent = `pwmconfig -c ${c} -s ${s}`;
  }
}

async function executePwmConfig() {
  const btnConfirm = document.getElementById("btn-confirm-pwm");
  const statusMsg = document.getElementById("pwm-status-msg");
  const channel = parseInt(document.getElementById("pwm-channel-input")?.value || "0", 10);
  const speed = parseInt(document.getElementById("pwm-speed-input")?.value || "255", 10);

  if (btnConfirm) {
    btnConfirm.disabled = true;
    btnConfirm.innerHTML = '⏳ Đang thực thi pwmconfig...';
  }

  if (statusMsg) {
    statusMsg.style.display = "block";
    statusMsg.style.background = "rgba(56, 189, 248, 0.15)";
    statusMsg.style.color = "#38bdf8";
    statusMsg.style.border = "1px solid rgba(56, 189, 248, 0.3)";
    statusMsg.textContent = `Đang chạy: pwmconfig -c ${channel} -s ${speed} ...`;
  }

  try {
    const res = await fetch("/api/system/pwmconfig", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ channel: channel, speed: speed })
    });
    const data = await res.json();

    if (res.ok) {
      if (statusMsg) {
        statusMsg.style.background = "rgba(52, 211, 153, 0.15)";
        statusMsg.style.color = "#34d399";
        statusMsg.style.border = "1px solid rgba(52, 211, 153, 0.3)";
        statusMsg.textContent = `✅ Thành công!\n${data.message || ""}`;
      }
      if (btnConfirm) {
        btnConfirm.disabled = false;
        btnConfirm.innerHTML = '<span>⚡</span> Chạy Lại Lệnh PWM';
      }
    } else {
      if (statusMsg) {
        statusMsg.style.background = "rgba(248, 113, 113, 0.15)";
        statusMsg.style.color = "#f87171";
        statusMsg.style.border = "1px solid rgba(248, 113, 113, 0.3)";
        let errText = `❌ Lỗi khi thực thi:\n${data.error || "Thực thi thất bại"}`;
        if (data.output && data.output !== data.error) {
          errText += `\n${data.output}`;
        }
        statusMsg.textContent = errText;
      }
      if (btnConfirm) {
        btnConfirm.disabled = false;
        btnConfirm.innerHTML = '<span>⚡</span> Thử Lại';
      }
    }
  } catch (err) {
    if (statusMsg) {
      statusMsg.style.background = "rgba(248, 113, 113, 0.15)";
      statusMsg.style.color = "#f87171";
      statusMsg.style.border = "1px solid rgba(248, 113, 113, 0.3)";
      statusMsg.textContent = `❌ Lỗi kết nối server: ${err.message}`;
    }
    if (btnConfirm) {
      btnConfirm.disabled = false;
      btnConfirm.innerHTML = '<span>⚡</span> Thử Lại';
    }
  }
}
