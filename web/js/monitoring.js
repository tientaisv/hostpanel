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
            data: []
          },
          {
            label: "Host RAM (%)",
            borderColor: "#6366f1",
            backgroundColor: "rgba(99, 102, 241, 0.1)",
            fill: false,
            tension: 0.3,
            data: []
          },
          {
            label: "Docker Total CPU (%)",
            borderColor: "#34d399",
            backgroundColor: "rgba(52, 211, 153, 0.1)",
            borderDash: [4, 4],
            fill: false,
            tension: 0.3,
            data: []
          },
          {
            label: "Docker Total RAM (%)",
            borderColor: "#a855f7",
            backgroundColor: "rgba(168, 85, 247, 0.1)",
            borderDash: [4, 4],
            fill: false,
            tension: 0.3,
            data: []
          }
        ]
      },
      options: {
        responsive: true,
        scales: {
          x: { grid: { color: "rgba(255,255,255,0.05)" }, ticks: { color: "#94a3b8" } },
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

function updateDashboardUI(data) {
  if (!data) return;

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

  // Total Docker Engine Resource Consumption
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

  const labels = [];
  const hostCpuData = [];
  const hostRamData = [];
  const dockerCpuData = [];
  const dockerRamData = [];

  records.forEach(rec => {
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
