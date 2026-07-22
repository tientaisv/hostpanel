let currentAIDiagID = null;

async function diagnoseContainerWithAI(id, name) {
  currentAIDiagID = id;
  const contentEl = document.getElementById("ai-diagnose-content");
  document.getElementById("ai-diagnose-title").textContent = `🤖 AI Incident Diagnostics - ${name}`;
  contentEl.innerHTML = "🔄 AI đang thu thập thông số & log để chẩn đoán nguyên nhân sự cố...";

  openModal("modal-ai-diagnose");

  try {
    const res = await fetch("/api/ai/diagnose", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ id })
    });
    if (!res.ok) {
      const err = await res.json();
      contentEl.innerHTML = `<span style="color: var(--accent-red);">❌ Lỗi AI Diagnostics: ${escapeHTML(err.error)}</span>`;
    } else {
      const data = await res.json();
      contentEl.innerHTML = renderFormattedAIResponse(data.diagnosis);
    }
  } catch (e) {
    contentEl.innerHTML = `<span style="color: var(--accent-red);">❌ Lỗi kết nối hệ thống: ${escapeHTML(e.message)}</span>`;
  }
}

function diagnoseLogsWithAI() {
  if (typeof currentLogContainerID !== "undefined" && currentLogContainerID) {
    diagnoseContainerWithAI(currentLogContainerID, "Container");
  }
}

async function runAIAuditSystem() {
  const box = document.getElementById("ai-response-box");
  box.innerHTML = "🛡️ AI đang phân tích toàn bộ thông số RAM, CPU, Disk, Swap và danh sách Containers của Host Server...";

  try {
    const res = await fetch("/api/ai/audit", { method: "POST" });
    if (!res.ok) {
      const err = await res.json();
      box.innerHTML = `<span style="color: var(--accent-red);">❌ Lỗi AI System Audit: ${escapeHTML(err.error)}</span>`;
    } else {
      const data = await res.json();
      box.innerHTML = renderFormattedAIResponse(data.audit);
    }
  } catch (e) {
    box.innerHTML = `<span style="color: var(--accent-red);">❌ Lỗi hệ thống: ${escapeHTML(e.message)}</span>`;
  }
}

async function sendCustomAIPrompt() {
  const input = document.getElementById("ai-prompt-input");
  const prompt = input.value.trim();
  if (!prompt) return;

  const box = document.getElementById("ai-response-box");
  box.innerHTML += `<div style="margin-top: 16px; padding: 12px; background: rgba(56, 189, 248, 0.1); border-radius: 8px; border-left: 3px solid var(--accent-blue);"><strong>👤 Bạn:</strong> ${escapeHTML(prompt)}</div>`;
  box.innerHTML += `<div id="ai-loading-msg" style="margin-top: 12px; color: var(--accent-blue);">🤖 AI đang suy nghĩ & phân tích hệ thống...</div>`;
  input.value = "";
  box.scrollTop = box.scrollHeight;

  try {
    const res = await fetch("/api/ai/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ prompt })
    });

    document.getElementById("ai-loading-msg")?.remove();

    if (!res.ok) {
      const err = await res.json();
      box.innerHTML += `<div style="color: var(--accent-red); margin-top: 8px;">❌ Lỗi AI: ${escapeHTML(err.error)}</div>`;
    } else {
      const data = await res.json();
      box.innerHTML += `<div style="margin-top: 12px;"><strong>🤖 AI Assistant:</strong><br>${renderFormattedAIResponse(data.response)}</div>`;
      box.scrollTop = box.scrollHeight;
    }
  } catch (e) {
    document.getElementById("ai-loading-msg")?.remove();
    box.innerHTML += `<div style="color: var(--accent-red); margin-top: 8px;">❌ Lỗi kết nối: ${escapeHTML(e.message)}</div>`;
  }
}

// Render formatted AI response with 1-Click "⚡ Run Command" buttons
function renderFormattedAIResponse(text) {
  if (!text) return "";

  // Parse ```bash ... ``` code blocks
  const codeBlockRegex = /```(?:bash|sh|shell)?\s*([\s\S]*?)```/g;

  let formatted = escapeHTML(text);

  formatted = formatted.replace(codeBlockRegex, (match, code) => {
    const rawCode = code.trim();
    const encodedCmd = btoa(encodeURIComponent(rawCode));
    return `
      <div style="background: #090d16; border: 1px solid var(--border-color); border-radius: 8px; padding: 12px; margin: 10px 0; font-family: monospace; font-size: 0.85rem; position: relative;">
        <pre style="margin: 0; white-space: pre-wrap; color: #38bdf8;">${escapeHTML(rawCode)}</pre>
        <button class="btn btn-primary" style="margin-top: 10px; padding: 4px 10px; font-size: 0.75rem;" onclick="runAIHostCommand('${encodedCmd}', this)">⚡ Execute Command on Host</button>
        <div class="cmd-exec-output" style="margin-top: 8px; display: none; background: rgba(0,0,0,0.5); padding: 8px; border-radius: 6px; font-size: 0.8rem;"></div>
      </div>
    `;
  });

  return formatted;
}

async function runAIHostCommand(encodedCmd, btnEl) {
  const command = decodeURIComponent(atob(encodedCmd));
  if (!confirm(`Bạn có chắc chắn muốn thực thi câu lệnh này trên Server không?\n\n$ ${command}`)) {
    return;
  }

  const outputDiv = btnEl.nextElementSibling;
  btnEl.disabled = true;
  btnEl.textContent = "⏳ Executing...";
  outputDiv.style.display = "block";
  outputDiv.innerHTML = `<span style="color: var(--accent-amber);">Đang thực thi lệnh trên Host...</span>`;

  try {
    const res = await fetch("/api/ai/exec_cmd", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ command })
    });

    const data = await res.json();
    btnEl.disabled = false;
    btnEl.textContent = "⚡ Execute Command on Host";

    if (!res.ok) {
      outputDiv.innerHTML = `<span style="color: var(--accent-red);">❌ Lỗi thực thi: ${escapeHTML(data.error)}</span>`;
    } else {
      const exitColor = data.exit_code === 0 ? "var(--accent-green)" : "var(--accent-red)";
      outputDiv.innerHTML = `
        <div style="color: ${exitColor}; font-weight: 700; margin-bottom: 4px;">Exit Code: ${data.exit_code} (${data.duration})</div>
        <pre style="margin: 0; color: #e2e8f0; max-height: 200px; overflow-y: auto;">${escapeHTML(data.output || "Completed with no output.")}</pre>
      `;
    }
  } catch (e) {
    btnEl.disabled = false;
    btnEl.textContent = "⚡ Execute Command on Host";
    outputDiv.innerHTML = `<span style="color: var(--accent-red);">❌ Lỗi kết nối: ${escapeHTML(e.message)}</span>`;
  }
}

document.getElementById("ai-prompt-input")?.addEventListener("keydown", (e) => {
  if (e.key === "Enter") {
    sendCustomAIPrompt();
  }
});
