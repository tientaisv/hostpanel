document.getElementById("login-form")?.addEventListener("submit", async (e) => {
  e.preventDefault();
  const userEl = document.getElementById("username");
  const passEl = document.getElementById("password");
  const errorEl = document.getElementById("error-msg");

  errorEl.style.display = "none";

  try {
    const res = await fetch("/api/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username: userEl.value.trim(),
        password: passEl.value
      })
    });

    const data = await res.json();
    if (!res.ok) {
      errorEl.textContent = data.error || "Đăng nhập thất bại.";
      errorEl.style.display = "block";
    } else {
      window.location.href = "/";
    }
  } catch (err) {
    errorEl.textContent = "Lỗi kết nối tới Server.";
    errorEl.style.display = "block";
  }
});
