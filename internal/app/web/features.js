/* Dashboard, device manager and application workspace bindings. */
function updateDashboardHealth() {
  const target = document.querySelector("#networkHealthText");
  if (!target) return;
  const online = devices.filter((device) => device.status === "online").length;
  const attention = devices.filter(
    (device) =>
      device.is_important &&
      ["offline", "suspected_offline"].includes(device.status),
  ).length;
  target.textContent = devices.length
    ? `${online} 台在线${attention ? ` · ${attention} 台长期在线设备已离线` : " · 长期在线设备状态良好"}`
    : "等待第一次网络扫描";
  target.classList.toggle("has-warning", attention > 0);
}

appExtensions.refreshed = function () {
  updateDashboardHealth();
};

appExtensions.activityLoaded = function () {
  const events = document.querySelectorAll("#statusEventList .activity-item");
  const scans = document.querySelectorAll("#scanHistoryList .activity-item");
  const eventCount = document.querySelector("#activityEventCount");
  const scanCount = document.querySelector("#activityScanCount");
  const latest = document.querySelector("#activityLatestTime");
  if (eventCount) eventCount.textContent = events.length;
  if (scanCount) scanCount.textContent = scans.length;
  const latestText =
    events[0]?.querySelector("small")?.textContent?.split(" · ").pop() ||
    scans[0]?.querySelector("small")?.textContent?.split(" · ")[0] ||
    "暂无";
  if (latest) latest.textContent = latestText;
};

appExtensions.managerRendered = function () {
  document
    .querySelector("#managerDialog .batch-bar")
    ?.classList.toggle("has-selection", selectedDevices.size > 0);
  document.querySelectorAll("#managerList .manager-device").forEach((row) => {
    const actions = row.querySelector(".manager-device-actions");
    const id = +(actions?.querySelector("[data-id]")?.dataset.id || 0);
    const device = devices.find((item) => item.id === id);
    if (!actions || !device) return;
    row.dataset.status = device.status || "unknown";
    if (device.is_important)
      row
        .querySelector(".manager-device-main b")
        ?.insertAdjacentHTML(
          "beforeend",
          '<span class="badge attention">长期在线</span>',
        );
    if (device.presence_mode === "occasional")
      row
        .querySelector(".manager-device-main b")
        ?.insertAdjacentHTML(
          "beforeend",
          '<span class="badge occasional">偶尔在线</span>',
        );
    if (device.is_flapping)
      row
        .querySelector(".manager-device-main b")
        ?.insertAdjacentHTML(
          "beforeend",
          '<span class="badge unstable">状态不稳定</span>',
        );
    if (can(permission.edit)) {
      actions.insertAdjacentHTML(
        "afterbegin",
        `<button data-action="attention" data-id="${id}">${device.is_important ? "取消长期在线" : "设为长期在线"}</button>`,
      );
      actions.querySelector('[data-action="attention"]').onclick = (event) =>
        managerAction(event.currentTarget);
      const extraActions = [
        ...actions.querySelectorAll('button:not([data-action="edit"])'),
      ];
      if (extraActions.length) {
        const menu = document.createElement("details");
        menu.className = "manager-actions-menu";
        menu.innerHTML =
          '<summary aria-label="更多设备操作">更多</summary><div></div>';
        extraActions.forEach((button) =>
          menu.querySelector("div").appendChild(button),
        );
        actions.appendChild(menu);
        menu.addEventListener("toggle", () => {
          if (!menu.open) return;
          document
            .querySelectorAll(".manager-actions-menu[open]")
            .forEach((other) => {
              if (other !== menu) other.removeAttribute("open");
            });
        });
      }
    }
    row.onclick = (event) => {
      if (event.target.closest("button,input,label,details,summary")) return;
      document.querySelector("#managerDialog").close();
      openDetail(id);
    };
  });
};

appExtensions.managerOpened = function () {
  document.querySelector("#managerDialog").scrollTop = 0;
};

appExtensions.managerAction = async function (button) {
  if (button.dataset.action !== "attention") return false;
  const id = +button.dataset.id;
  const device = devices.find((item) => item.id === id);
  if (!device) return true;
  const wasImportant = device.is_important;
  try {
    await api(`/api/devices/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ is_important: !wasImportant }),
    });
    await refresh();
    renderManager();
    toast(wasImportant ? "已取消长期在线标记" : "已设为长期在线设备");
  } catch (error) {
    toast(error.message);
  }
  return true;
};

async function loadMaintenanceStatus() {
  const result = document.querySelector("#maintenanceStatus");
  if (!result) return;
  try {
    const status = await api("/api/maintenance");
    if (!status.backup_count) {
      result.textContent = "服务器上还没有自动备份。";
      return;
    }
    const last = new Date(status.backups[0].created_at).toLocaleString();
    result.textContent = `服务器上有 ${status.backup_count} 份备份，最近一份：${last}`;
  } catch (error) {
    result.textContent = error.message;
  }
}

function ensureVendorDatabaseUI() {
  const pane = document.querySelector("#aboutPane");
  if (!pane || document.querySelector("#vendorDatabaseCard")) return;
  pane.insertAdjacentHTML(
    "beforeend",
    `
    <section id="vendorDatabaseCard" class="settings-group vendor-database-card">
      <div><b>MAC 厂商资料</b><small id="vendorDatabaseStatus">正在读取状态…</small></div>
      <button type="button" id="vendorDatabaseUpdate">从 IEEE 官方更新</button>
      <p class="muted">资料只保存在本机，用来识别网卡厂商；手机的随机 MAC 不参与厂商判断。</p>
    </section>`,
  );
  document.querySelector("#vendorDatabaseUpdate").onclick = async () => {
    const button = document.querySelector("#vendorDatabaseUpdate");
    button.disabled = true;
    button.textContent = "正在更新…";
    try {
      const status = await api("/api/vendor-database/update", {
        method: "POST",
      });
      showVendorDatabaseStatus(status);
      toast(`厂商资料已更新，共 ${status.entries} 条`);
    } catch (error) {
      toast(error.message);
    } finally {
      button.disabled = false;
      button.textContent = "从 IEEE 官方更新";
    }
  };
}

function showVendorDatabaseStatus(status) {
  const target = document.querySelector("#vendorDatabaseStatus");
  if (!target) return;
  target.textContent = status.available
    ? `已有 ${status.entries} 条资料 · 更新于 ${new Date(status.updated_at).toLocaleString()}`
    : "尚未下载，基础扫描不受影响";
}

async function refreshVendorDatabaseStatus() {
  try {
    showVendorDatabaseStatus(await api("/api/vendor-database"));
  } catch (error) {
    const target = document.querySelector("#vendorDatabaseStatus");
    if (target) target.textContent = error.message;
  }
}

appExtensions.bind = function () {
  ensureMaintenanceUI();
  ensureQualityTools();
  ensureVendorDatabaseUI();
  updateDashboardHealth();
  document.querySelector("#homeBtn").onclick = () => cy?.fit(undefined, 60);
  document.querySelector("#canvasHelpClose").onclick = () =>
    document.querySelector("#canvasHelpDialog").close();
  document.querySelector("#mobileSelectBtn").onclick = () =>
    setMobileSelectionMode(!mobileSelectionMode);
  document.querySelector("#canvasHelpBtn").onclick = () =>
    document.querySelector("#canvasHelpDialog").showModal();
  document.querySelector("#clearSelectionBtn").onclick = () => {
    cy?.nodes().unselect();
    setMobileSelectionMode(false);
  };
  document
    .querySelectorAll("#selectionBar [data-align]")
    .forEach(
      (button) =>
        (button.onclick = () => alignSelectedNodes(button.dataset.align)),
    );
  document.addEventListener("keydown", (event) => {
    if (
      ["INPUT", "TEXTAREA", "SELECT"].includes(document.activeElement?.tagName)
    )
      return;
    if (event.key === "Escape") {
      closeTopologyMenu();
      quickLinkSource = null;
      cy?.nodes().removeClass("link-source");
      cy?.nodes().unselect();
      setMobileSelectionMode(false);
    }
    if (
      (event.ctrlKey || event.metaKey) &&
      event.key.toLowerCase() === "a" &&
      cy &&
      can(permission.edit)
    ) {
      event.preventDefault();
      cy.nodes(":visible").select();
      updateSelectionBar();
    }
    if (event.key === "?" && !event.ctrlKey && !event.metaKey && !event.altKey)
      document.querySelector("#canvasHelpDialog")?.showModal();
  });
  document.querySelector("#aboutTab").onclick = async () => {
    showPane("about");
    const [result] = await Promise.all([
      api("/api/version"),
      refreshVendorDatabaseStatus(),
    ]);
    const value = result.version || "dev";
    document.querySelector("#versionText").textContent = value.startsWith(
      "dev-",
    )
      ? `开发版 · ${value.slice(4)}`
      : value === "dev"
        ? "开发版 · 本地构建"
        : `版本 ${value}`;
  };
  const overview = document.querySelector("#overviewNav");
  const workspaceMeta = {
    overview: ["家庭网络中心", "网络拓扑"],
    devices: ["设备与连接", "设备管理"],
    activity: ["扫描与状态变化", "运行记录"],
    settings: ["扫描、通知与数据", "系统设置"],
    account: ["成员与访问权限", "账户管理"],
  };
  const navPages = [
    [
      document.querySelector("#manageBtn"),
      document.querySelector("#managerClose"),
    ],
    [
      document.querySelector("#activityBtn"),
      document.querySelector("#activityClose"),
    ],
    [
      document.querySelector("#settingsBtn"),
      document.querySelector('#settingsForm button[value="cancel"]'),
    ],
    [
      document.querySelector("#accountBtn"),
      document.querySelector("#accountClose"),
    ],
  ];
  const workspaceDialogs = [
    "managerDialog",
    "activityDialog",
    "settingsDialog",
    "accountDialog",
  ];
  const closeWorkspacePages = () =>
    workspaceDialogs.forEach((id) => {
      const dialog = document.querySelector(`#${id}`);
      if (dialog?.open) dialog.close();
    });
  const activateNav = (
    button,
    workspace = button?.dataset.workspace || "overview",
  ) => {
    document.querySelectorAll(".sidebar-nav .nav-button").forEach((item) => {
      const active = item === button;
      item.classList.toggle("active", active);
      if (active) item.setAttribute("aria-current", "page");
      else item.removeAttribute("aria-current");
    });
    const [eyebrow, title] = workspaceMeta[workspace] || workspaceMeta.overview;
    document.querySelector("#pageEyebrow").textContent = eyebrow;
    document.querySelector("#pageTitle").textContent = title;
    document.title = `${title} · MeowTopo 喵拓`;
    document.body.dataset.workspace = workspace;
    document.body.classList.toggle("workspace-open", workspace !== "overview");
    document
      .querySelector("#accountBtn")
      ?.classList.toggle("active", workspace === "account");
  };
  navPages.forEach(([open, close]) => {
    if (open) {
      const action = open.onclick;
      open.onclick = async (event) => {
        closeWorkspacePages();
        activateNav(open, open.dataset.workspace);
        const result = await action?.call(open, event);
        if (
          workspaceDialogs.some((id) => document.querySelector(`#${id}`)?.open)
        )
          activateNav(open, open.dataset.workspace);
        return result;
      };
    }
    if (close) {
      const action = close.onclick;
      close.onclick = (event) => {
        activateNav(overview);
        return action?.call(close, event);
      };
    }
  });
  workspaceDialogs.forEach((id) =>
    document.querySelector(`#${id}`)?.addEventListener("close", () => {
      setTimeout(() => {
        if (
          !workspaceDialogs.some(
            (dialogID) => document.querySelector(`#${dialogID}`)?.open,
          )
        )
          activateNav(overview);
      }, 0);
    }),
  );
  if (overview)
    overview.onclick = () => {
      closeWorkspacePages();
      activateNav(overview);
      cy?.fit(undefined, 60);
    };
  document.querySelectorAll(".status-card").forEach((card) => {
    const openFilteredDevices = () => {
      document.querySelector("#manageBtn").click();
      document.querySelector("#managerStatus").value = card.dataset.status;
      renderManager();
    };
    card.onclick = openFilteredDevices;
    card.onkeydown = (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        openFilteredDevices();
      }
    };
  });
  const accountButton = document.querySelector("#accountBtn");
  if (accountButton) {
    const action = accountButton.onclick;
    accountButton.onclick = async (event) => {
      closeWorkspacePages();
      activateNav(null, "account");
      return action?.call(accountButton, event);
    };
  }
  document.querySelector("#managerClose").textContent = "返回拓扑";
  document.querySelector("#accountClose").textContent = "返回拓扑";
  const pageDialogs = [
    ["managerDialog", "设备管理"],
    ["activityDialog", "运行记录"],
    ["settingsDialog", "系统设置"],
    ["accountDialog", "账户管理"],
  ];
  pageDialogs.forEach(([id, label]) => {
    const dialog = document.querySelector(`#${id}`);
    const heading = dialog?.querySelector("h2");
    if (!dialog || !heading) return;
    heading.id ||= `${id}Title`;
    dialog.setAttribute("aria-labelledby", heading.id);
    dialog.setAttribute("aria-label", label);
  });
  document
    .querySelectorAll("#settingsDialog .tabs, #activityDialog .activity-tabs")
    .forEach((tablist) => {
      tablist.setAttribute("role", "tablist");
      tablist.querySelectorAll("button").forEach((button) => {
        button.setAttribute("role", "tab");
        button.setAttribute(
          "aria-selected",
          String(button.classList.contains("active")),
        );
        button.addEventListener("click", () => {
          tablist
            .querySelectorAll("button")
            .forEach((item) =>
              item.setAttribute("aria-selected", String(item === button)),
            );
        });
      });
    });
  document.querySelectorAll("[data-activity-pane]").forEach(
    (button) =>
      (button.onclick = () => {
        document
          .querySelectorAll("[data-activity-pane]")
          .forEach((item) => item.classList.toggle("active", item === button));
        document
          .querySelectorAll("[data-activity-content]")
          .forEach((pane) =>
            pane.classList.toggle(
              "hidden",
              pane.dataset.activityContent !== button.dataset.activityPane,
            ),
          );
      }),
  );
  const createForm = document.querySelector("#accountCreateForm");
  const permissionGrid = createForm?.querySelector(".permission-grid");
  if (permissionGrid && !document.querySelector("#accountRolePreset")) {
    permissionGrid.insertAdjacentHTML(
      "beforebegin",
      `<label class="role-preset">使用权限<select id="accountRolePreset"><option value="view">只查看（适合访客）</option><option value="family">家庭成员（可整理设备）</option><option value="maintain">协助管理（可扫描和设置）</option><option value="admin">管理员（全部权限）</option></select><small>先选择常用权限，需要时再在下方微调。</small></label>`,
    );
    document.querySelector("#accountRolePreset").onchange = (event) => {
      const role = event.target.value;
      document.querySelector("#newAdmin").checked = role === "admin";
      document.querySelector("#newCanEdit").checked = [
        "family",
        "maintain",
        "admin",
      ].includes(role);
      document.querySelector("#newCanScan").checked = [
        "maintain",
        "admin",
      ].includes(role);
      document.querySelector("#newCanSettings").checked = [
        "maintain",
        "admin",
      ].includes(role);
      document
        .querySelectorAll(".permission-grid input")
        .forEach((input) => (input.disabled = role === "admin"));
    };
  }
  [
    document.querySelector("#wizardLater"),
    document.querySelector('#settingsForm button[value="cancel"]'),
    document.querySelector('#manualForm button[value="cancel"]'),
  ]
    .filter(Boolean)
    .forEach((button) => {
      button.type = "button";
      button.onclick = () => {
        const form = button.closest("form");
        form?.reset();
        button.closest("dialog")?.close("cancel");
      };
    });
  const restore = document.querySelector("#restoreFile");
  if (!restore) return;
  restore.onchange = async (event) => {
    const file = event.target.files[0];
    if (!file || !confirm("恢复会替换当前数据库，是否继续？")) return;
    try {
      const response = await fetch("/api/restore", {
        method: "POST",
        headers: {
          "Content-Type": "application/zip",
          "X-CSRF-Token": csrfToken,
        },
        body: file,
      });
      if (!response.ok) throw new Error((await response.json()).error.message);
      toast("恢复完成，请重新登录");
      setTimeout(() => location.reload(), 1000);
    } catch (error) {
      toast(error.message);
    }
  };
};
