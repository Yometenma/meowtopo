const $ = (s) => document.querySelector(s);
let cy,
  devices = [],
  connections = [],
  settings = {},
  saveTimers = new Map(),
  selectedDevices = new Set(),
  managerFiltered = [],
  currentAccount = null,
  csrfToken = "";
const permission = { view: 1, edit: 2, scan: 4, settings: 8, users: 16 };
const appExtensions = {
  deviceLabel: () => "",
  deviceDetail: () => {},
  saveDeviceDetail: null,
  notificationValues: () => ({}),
  notificationUI: () => {},
  settingsOpened: async () => {},
  snmpOpened: async () => {},
  snmpBind: () => {},
  topologyReady: () => {},
  refreshed: () => {},
  activityLoaded: () => {},
  managerRendered: () => {},
  managerOpened: () => {},
  managerAction: async () => false,
  bind: () => {},
};
const icons = {
  internet: "☁",
  modem: "◈",
  gateway: "⌂",
  router: "⌂",
  ap: "◉",
  switch: "⇄",
  nas: "▣",
  linux: "⌘",
  windows: "⊞",
  macos: "●",
  phone: "▯",
  tablet: "▭",
  tv: "▰",
  camera: "◉",
  iot: "⌁",
  game: "◆",
  printer: "▤",
  room: "⌂",
  unknown: "?",
};
async function api(url, opt = {}) {
  const method = (opt.method || "GET").toUpperCase(),
    headers = { "Content-Type": "application/json", ...(opt.headers || {}) };
  if (!["GET", "HEAD", "OPTIONS"].includes(method) && csrfToken)
    headers["X-CSRF-Token"] = csrfToken;
  const r = await fetch(url, { ...opt, headers, credentials: "same-origin" });
  const ct = r.headers.get("content-type") || "";
  const data = ct.includes("json") ? await r.json() : await r.blob();
  if (!r.ok) {
    const e = new Error(data?.error?.message || `请求失败 (${r.status})`);
    e.status = r.status;
    e.code = data?.error?.code;
    throw e;
  }
  return data;
}
function can(bit) {
  return (
    !!currentAccount &&
    (currentAccount.is_admin || (currentAccount.permissions & bit) === bit)
  );
}
function applyAccountPermissions() {
  if (!currentAccount) return;
  $("#accountBtn").classList.remove("hidden");
  $("#accountBtn").textContent =
    currentAccount.display_name || currentAccount.username;
  $("#scanBtn").classList.toggle("hidden", !can(permission.scan));
  $("#manageBtn").classList.remove("hidden");
  $("#addBtn").classList.toggle("hidden", !can(permission.edit));
  $("#layoutBtn").classList.toggle("hidden", !can(permission.edit));
  $("#settingsBtn").classList.toggle("hidden", !can(permission.settings));
  document.body.classList.toggle("read-only", !can(permission.edit));
  document.body.classList.toggle("no-scan", !can(permission.scan));
  if (cy) {
    cy.autoungrabify(!can(permission.edit));
    cy.nodes().forEach((n) =>
      can(permission.edit) ? n.grabify() : n.ungrabify(),
    );
  }
}
function authResult(data) {
  currentAccount = data.user;
  csrfToken = data.csrf_token;
  applyAccountPermissions();
}
function openAuth(setup, resolve) {
  const dialog = $("#authDialog");
  $("#authTitle").textContent = setup ? "创建管理员账户" : "登录 MeowTopo";
  $("#authIntro").textContent = setup
    ? "这是首次启动。请创建管理家庭网络的首位管理员。"
    : "登录后查看家里的网络状态。";
  $("#authDisplayLabel").classList.toggle("hidden", !setup);
  $("#authConfirmLabel").classList.toggle("hidden", !setup);
  $("#authSubmit").textContent = setup ? "创建管理员" : "登录";
  $("#authPassword").autocomplete = setup ? "new-password" : "current-password";
  $("#authError").textContent = "";
  dialog.oncancel = (e) => e.preventDefault();
  $("#authForm").onsubmit = async (e) => {
    e.preventDefault();
    const password = $("#authPassword").value;
    if (setup && password !== $("#authConfirmPassword").value) {
      $("#authError").textContent = "两次输入的密码不一致";
      return;
    }
    const payload = { username: $("#authUsername").value, password };
    if (setup) payload.displayName = $("#authDisplayName").value;
    try {
      const data = await api(
        setup ? "/api/auth/bootstrap" : "/api/auth/login",
        { method: "POST", body: JSON.stringify(payload) },
      );
      authResult(data);
      dialog.close();
      resolve();
    } catch (x) {
      $("#authError").textContent = x.message;
    }
  };
  dialog.showModal();
}
async function requireAccount() {
  const status = await api("/api/auth/status");
  if (!status.setup_required) {
    try {
      authResult(await api("/api/auth/me"));
      return;
    } catch (e) {
      if (e.status !== 401) throw e;
    }
  }
  await new Promise((resolve) => openAuth(status.setup_required, resolve));
}
function permissionText(u) {
  if (u.is_admin) return "管理员 · 全部权限";
  const list = ["查看"];
  if (u.permissions & permission.edit) list.push("修改设备");
  if (u.permissions & permission.scan) list.push("执行扫描");
  if (u.permissions & permission.settings) list.push("修改设置");
  return list.join(" · ");
}
async function renderAccounts() {
  if (!can(permission.users)) return;
  const users = await api("/api/users");
  $("#accountList").innerHTML = users
    .map(
      (u) =>
        `<div class="account-row" data-user="${u.id}"><div><b>${esc(u.display_name)} <span class="muted">@${esc(u.username)}</span></b><small>${permissionText(u)} · ${u.is_active ? "已启用" : "已停用"}</small></div><div class="account-actions"><label class="check"><input data-field="edit" type="checkbox" ${u.permissions & permission.edit ? "checked" : ""} ${u.is_admin ? "disabled" : ""}>改设备</label><label class="check"><input data-field="scan" type="checkbox" ${u.permissions & permission.scan ? "checked" : ""} ${u.is_admin ? "disabled" : ""}>可扫描</label><label class="check"><input data-field="settings" type="checkbox" ${u.permissions & permission.settings ? "checked" : ""} ${u.is_admin ? "disabled" : ""}>改设置</label><label class="check"><input data-field="admin" type="checkbox" ${u.is_admin ? "checked" : ""}>管理员</label><label class="check"><input data-field="active" type="checkbox" ${u.is_active ? "checked" : ""}>启用</label><button data-account-save>保存</button><button data-account-password>重置密码</button></div></div>`,
    )
    .join("");
  $("#accountList")
    .querySelectorAll("[data-account-save]")
    .forEach(
      (button) =>
        (button.onclick = async () => {
          const row = button.closest("[data-user]"),
            get = (f) => row.querySelector(`[data-field="${f}"]`).checked;
          let permissions = permission.view;
          if (get("edit")) permissions |= permission.edit;
          if (get("scan")) permissions |= permission.scan;
          if (get("settings")) permissions |= permission.settings;
          try {
            await api(`/api/users/${row.dataset.user}`, {
              method: "PATCH",
              body: JSON.stringify({
                permissions,
                is_admin: get("admin"),
                is_active: get("active"),
              }),
            });
            toast("账户权限已保存");
            await renderAccounts();
          } catch (e) {
            toast(e.message);
          }
        }),
    );
  $("#accountList")
    .querySelectorAll("[data-account-password]")
    .forEach(
      (button) =>
        (button.onclick = async () => {
          const password = prompt("输入至少 10 个字符的新密码");
          if (!password) return;
          try {
            await api(
              `/api/users/${button.closest("[data-user]").dataset.user}`,
              { method: "PATCH", body: JSON.stringify({ password }) },
            );
            toast("密码已重置，其他登录已退出");
          } catch (e) {
            toast(e.message);
          }
        }),
    );
}
async function openAccount() {
  const d = $("#accountDialog");
  $("#currentAccountText").textContent =
    `${currentAccount.display_name}（${currentAccount.username}）· ${permissionText(currentAccount)}`;
  $("#accountAdminPane").classList.toggle("hidden", !can(permission.users));
  d.show();
  if (can(permission.users)) await renderAccounts();
}
async function createAccount(e) {
  e.preventDefault();
  let permissions = permission.view;
  if ($("#newCanEdit").checked) permissions |= permission.edit;
  if ($("#newCanScan").checked) permissions |= permission.scan;
  if ($("#newCanSettings").checked) permissions |= permission.settings;
  try {
    await api("/api/users", {
      method: "POST",
      body: JSON.stringify({
        username: $("#newUsername").value,
        displayName: $("#newDisplayName").value,
        password: $("#newPassword").value,
        permissions,
        is_admin: $("#newAdmin").checked,
      }),
    });
    e.target.reset();
    $("#accountCreateError").textContent = "";
    toast("账户已创建");
    await renderAccounts();
  } catch (x) {
    $("#accountCreateError").textContent = x.message;
  }
}
function bindAccount() {
  $("#accountBtn").onclick = openAccount;
  $("#accountClose").onclick = () => $("#accountDialog").close();
  $("#accountCreateForm").onsubmit = createAccount;
  $("#newAdmin").onchange = (e) =>
    document.querySelectorAll(".permission-grid input").forEach((input) => {
      input.disabled = e.target.checked;
      if (e.target.checked) input.checked = true;
    });
  $("#logoutBtn").onclick = async () => {
    try {
      await api("/api/auth/logout", { method: "POST" });
      location.reload();
    } catch (e) {
      toast(e.message);
    }
  };
}
function nameOf(d) {
  return d.user_name || d.auto_hostname || d.current_ip || "未命名设备";
}
function typeOf(d) {
  const type = d.user_device_type || d.auto_device_type || "unknown";
  return type === "camera" ? "iot" : type;
}
function toast(t) {
  const e = $("#toast");
  e.textContent = t;
  e.classList.add("show");
  clearTimeout(e.t);
  e.t = setTimeout(() => e.classList.remove("show"), 2600);
}
