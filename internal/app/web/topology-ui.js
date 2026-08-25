/* Topology workspace shell and direct canvas interactions. */
function shellIcon(name) {
  const paths = {
    home: '<path d="M3 11.5 12 4l9 7.5M5.5 10v10h13V10M9 20v-6h6v6"/>',
    devices:
      '<rect x="3" y="4" width="18" height="13" rx="3"/><path d="M8 21h8m-4-4v4M7 9h.01m4 0h6"/>',
    activity: '<path d="M4 19V9m5 10V5m5 14v-7m5 7V3"/>',
    settings:
      '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1a1.7 1.7 0 0 0 1.9.3A1.7 1.7 0 0 0 10 3V2.8h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z"/>',
    layout:
      '<circle cx="6" cy="6" r="2"/><circle cx="18" cy="6" r="2"/><circle cx="12" cy="18" r="2"/><path d="m7.7 7.1 3.2 8.8m5.4-8.8-3.2 8.8M8 6h8"/>',
    fit: '<path d="M8 3H3v5m13-5h5v5M8 21H3v-5m13 5h5v-5"/>',
    select:
      '<path d="M4 8V4h4m8 0h4v4M4 16v4h4m8 0h4v-4"/><path d="M9 9h6v6H9z"/>',
  };
  return `<svg viewBox="0 0 24 24" aria-hidden="true">${paths[name]}</svg>`;
}
function ensureCanvasHelpUI() {
  if (document.querySelector("#canvasHelpDialog")) return;
  document.body.insertAdjacentHTML(
    "beforeend",
    `<dialog id="canvasHelpDialog" class="canvas-help-dialog">
    <div class="canvas-help-head"><div><span class="page-kicker">TOPOLOGY GUIDE</span><h2 id="canvasHelpTitle">拓扑操作指南</h2><p>不用切换编辑模式，直接在画布上操作。</p></div><button type="button" id="canvasHelpClose">关闭</button></div>
    <div class="canvas-help-platforms">
      <section><b>Windows</b><dl><dt>移动画布</dt><dd>按住空白处拖动</dd><dt>缩放</dt><dd>滚轮</dd><dt>框选多个</dt><dd>Shift + 拖动空白处</dd><dt>快捷操作</dt><dd>右键设备或连接线</dd><dt>全选</dt><dd>Ctrl + A</dd></dl></section>
      <section><b>macOS</b><dl><dt>移动画布</dt><dd>按住空白处拖动</dd><dt>缩放</dt><dd>触控板捏合或滚动</dd><dt>框选多个</dt><dd>Shift + 拖动空白处</dd><dt>快捷操作</dt><dd>双指点按设备或连接线</dd><dt>全选</dt><dd>⌘ + A</dd></dl></section>
      <section><b>手机与平板</b><dl><dt>移动画布</dt><dd>单指拖动空白处</dd><dt>缩放</dt><dd>双指捏合</dd><dt>选择多个</dt><dd>点“选择”，再逐个点设备</dd><dt>快捷操作</dt><dd>长按设备或连接线</dd><dt>移动一组</dt><dd>拖动任意已选设备</dd></dl></section>
    </div>
    <section class="connection-guide"><h3>连接方式代表什么</h3><div><article><b>网线</b><p>确认设备之间存在有线连接。</p></article><article><b>Wi-Fi</b><p>确认设备通过无线网络接入上级。</p></article><article><b>逻辑连接</b><p>表示流量、路由或服务上的上下级，不宣称真实网线接法。</p></article><article><b>虚拟连接</b><p>用于虚拟机、容器、虚拟网桥等软件关系。</p></article><article><b>未知连接</b><p>知道设备有关联，但目前不能确定连接方式。</p></article><article><b>虚线推测</b><p>由喵拓根据有限信息推测，可由你手动确认或修改。</p></article></div></section>
  </dialog>`,
  );
  const dialog = document.querySelector("#canvasHelpDialog");
  dialog.setAttribute("aria-labelledby", "canvasHelpTitle");
  document.querySelector("#canvasHelpClose").onclick = () => dialog.close();
}

function ensureDashboardChrome() {
  if (document.querySelector("#appSidebar")) return;
  document.body.classList.add("dashboard-shell");
  const header = document.querySelector(".topbar");
  const main = document.querySelector("main");
  const sidebar = document.createElement("aside");
  sidebar.id = "appSidebar";
  sidebar.className = "app-sidebar";
  sidebar.innerHTML =
    '<nav class="sidebar-nav"></nav><div class="sidebar-foot"><span class="sidebar-pulse"></span><span>本地运行中</span></div>';
  document.querySelector("#app").insertBefore(sidebar, header);

  const brand = document.querySelector("#homeBtn");
  sidebar.insertBefore(brand, sidebar.firstChild);
  brand.onclick = () => cy?.fit(undefined, 60);
  const nav = sidebar.querySelector(".sidebar-nav");
  const entries = [
    [document.querySelector("#manageBtn"), "devices", "设备管理"],
    [document.querySelector("#activityBtn"), "activity", "运行记录"],
    [document.querySelector("#settingsBtn"), "settings", "系统设置"],
  ];
  entries.forEach(([button, icon, label]) => {
    button.dataset.workspace = icon;
    button.setAttribute("aria-label", label);
    button.classList.remove("primary");
    button.classList.add("nav-button");
    button.innerHTML = `${shellIcon(icon)}<span>${label}</span>`;
    nav.appendChild(button);
  });
  nav.insertAdjacentHTML(
    "afterbegin",
    `<button type="button" class="nav-button active" id="overviewNav" data-workspace="overview" aria-label="拓扑总览" aria-current="page">${shellIcon("home")}<span>拓扑总览</span></button>`,
  );
  document.querySelector("#overviewNav").onclick = () => cy?.fit(undefined, 60);

  header.insertAdjacentHTML(
    "afterbegin",
    '<div class="page-heading" aria-live="polite"><small id="pageEyebrow">家庭网络中心</small><strong id="pageTitle">网络拓扑</strong></div>',
  );
  main.querySelector("#canvasPage").insertAdjacentHTML(
    "beforeend",
    `
    <section class="canvas-context" aria-label="拓扑状态">
      <span class="context-kicker"><i></i> LIVE TOPOLOGY</span>
      <strong>家庭网络拓扑</strong>
      <small id="networkHealthText">正在读取网络状态…</small>
    </section>`,
  );
  const toolbar = document.createElement("div");
  toolbar.className = "canvas-toolbar";
  main.querySelector("#canvasPage").appendChild(toolbar);
  const layout = document.querySelector("#layoutBtn"),
    fit = document.querySelector("#fitBtn");
  layout.innerHTML = `${shellIcon("layout")}<span>整理布局</span>`;
  fit.innerHTML = `${shellIcon("fit")}<span>适应画布</span>`;
  toolbar.append(layout, fit);
  toolbar.insertAdjacentHTML(
    "beforeend",
    '<button type="button" id="canvasHelpBtn" title="查看拓扑操作指南"><b class="help-symbol">?</b><span>使用帮助</span></button>',
  );
  toolbar.insertAdjacentHTML(
    "afterbegin",
    `<button type="button" id="mobileSelectBtn" class="mobile-select-button${can(permission.edit) ? "" : " hidden"}" title="选择多个设备">${shellIcon("select")}<span>选择</span></button>`,
  );
  main
    .querySelector("#canvasPage")
    .insertAdjacentHTML(
      "beforeend",
      `<div id="selectionBar" class="selection-bar hidden"><b><span id="selectionCount">0</span> 台已选</b><button type="button" data-align="horizontal">横向对齐</button><button type="button" data-align="vertical">纵向对齐</button><button type="button" id="clearSelectionBtn">取消选择</button></div>`,
    );
  document
    .querySelector(".legend")
    ?.insertAdjacentHTML(
      "beforeend",
      '<span class="muted selection-hint">Shift + 拖动框选</span>',
    );
  updateDashboardHealth();
  ensureCanvasHelpUI();

  const statusCards = [
    [
      document.querySelector("#onlineN")?.closest("span"),
      "online",
      "查看在线设备",
    ],
    [
      document.querySelector("#suspectN")?.closest("span"),
      "suspected_offline",
      "查看疑似离线设备",
    ],
    [
      document.querySelector("#offlineN")?.closest("span"),
      "offline",
      "查看离线设备",
    ],
    [document.querySelector("#totalN")?.closest("span"), "", "查看全部设备"],
  ];
  statusCards.forEach(([card, status, label]) => {
    if (!card) return;
    card.classList.add("status-card");
    card.dataset.status = status;
    card.tabIndex = 0;
    card.setAttribute("role", "button");
    card.setAttribute("aria-label", label);
  });
}

let quickLinkSource = null;
let mobileSelectionMode = false;

function closeTopologyMenu() {
  document.querySelector("#topologyMenu")?.remove();
}

function updateSelectionBar() {
  const count = cy ? cy.nodes(":selected").length : 0;
  const bar = document.querySelector("#selectionBar");
  if (!bar) return;
  document.querySelector("#selectionCount").textContent = count;
  bar.classList.toggle("hidden", count < 2);
}

function setMobileSelectionMode(enabled) {
  mobileSelectionMode = enabled && can(permission.edit);
  document
    .querySelector("#mobileSelectBtn")
    ?.classList.toggle("active", mobileSelectionMode);
  if (mobileSelectionMode)
    toast("逐个点选设备，再拖动任一已选设备即可整组移动");
}

function alignSelectedNodes(axis) {
  if (!cy || !can(permission.edit)) return;
  const selected = cy.nodes(":selected").filter((node) => !node.locked());
  if (selected.length < 2) return;
  const values = selected.map((node) =>
    axis === "horizontal" ? node.position("y") : node.position("x"),
  );
  const center = values.reduce((sum, value) => sum + value, 0) / values.length;
  selected.forEach((node) => {
    node.position(
      axis === "horizontal"
        ? { x: node.position("x"), y: center }
        : { x: center, y: node.position("y") },
    );
    savePosition(node);
  });
  toast(axis === "horizontal" ? "已横向对齐选中设备" : "已纵向对齐选中设备");
}

function installDirectBoxSelection() {
  const container = document.querySelector("#cy");
  if (!container || container.dataset.boxSelectionReady) return;
  container.dataset.boxSelectionReady = "true";
  container.addEventListener(
    "mousedown",
    (event) => {
      if (!event.shiftKey || event.button !== 0 || !cy || !can(permission.edit))
        return;
      const containerRect = container.getBoundingClientRect();
      const x = event.clientX - containerRect.left;
      const y = event.clientY - containerRect.top;
      const overNode = cy.nodes(":visible").some((node) => {
        const box = node.renderedBoundingBox({
          includeLabels: true,
          includeOverlays: true,
        });
        return x >= box.x1 && x <= box.x2 && y >= box.y1 && y <= box.y2;
      });
      if (overNode) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      const page = document.querySelector("#canvasPage");
      const pageRect = page.getBoundingClientRect();
      const startX = event.clientX - pageRect.left;
      const startY = event.clientY - pageRect.top;
      const marquee = document.createElement("div");
      marquee.className = "selection-marquee";
      page.appendChild(marquee);
      const move = (moveEvent) => {
        const currentX = moveEvent.clientX - pageRect.left;
        const currentY = moveEvent.clientY - pageRect.top;
        marquee.style.left = `${Math.min(startX, currentX)}px`;
        marquee.style.top = `${Math.min(startY, currentY)}px`;
        marquee.style.width = `${Math.abs(currentX - startX)}px`;
        marquee.style.height = `${Math.abs(currentY - startY)}px`;
      };
      const finish = (upEvent) => {
        window.removeEventListener("mousemove", move, true);
        window.removeEventListener("mouseup", finish, true);
        const endX = upEvent.clientX - containerRect.left;
        const endY = upEvent.clientY - containerRect.top;
        const left = Math.min(x, endX),
          right = Math.max(x, endX);
        const top = Math.min(y, endY),
          bottom = Math.max(y, endY);
        cy.nodes(":visible").forEach((node) => {
          const position = node.renderedPosition();
          if (
            position.x >= left &&
            position.x <= right &&
            position.y >= top &&
            position.y <= bottom
          )
            node.select();
        });
        marquee.remove();
        updateSelectionBar();
      };
      move(event);
      window.addEventListener("mousemove", move, true);
      window.addEventListener("mouseup", finish, true);
    },
    true,
  );
}

async function createQuickConnection(targetID, connectionType) {
  if (!quickLinkSource || quickLinkSource === targetID) return;
  try {
    await api(`/api/devices/${targetID}/connections`, {
      method: "POST",
      body: JSON.stringify({
        parent_id: quickLinkSource,
        connection_type: connectionType,
        port_label: "",
      }),
    });
    const source = devices.find((device) => device.id === quickLinkSource);
    const target = devices.find((device) => device.id === targetID);
    quickLinkSource = null;
    cy.nodes().removeClass("link-source");
    closeTopologyMenu();
    await refresh();
    toast(
      `已连接：${source ? nameOf(source) : "上级设备"} → ${target ? nameOf(target) : "目标设备"}`,
    );
  } catch (error) {
    toast(error.message);
  }
}

function openTopologyMenu(node, renderedPosition) {
  closeTopologyMenu();
  const id = +node.id();
  const editable = can(permission.edit);
  const menu = document.createElement("div");
  menu.id = "topologyMenu";
  menu.className = "topology-menu";
  const linkChoices =
    quickLinkSource && quickLinkSource !== id
      ? `<div class="topology-menu-title">连接到 ${esc(nameOf(devices.find((device) => device.id === id) || { user_name: "此设备" }))}</div><button data-link-type="ethernet">网线连接</button><button data-link-type="wifi">Wi-Fi 连接</button><button data-link-type="logical">逻辑连接</button><button data-link-type="virtual">虚拟连接</button><button data-cancel-link>取消连线</button>`
      : `${editable ? "<button data-start-link>从此设备开始连线</button>" : ""}<button data-toggle-select>${node.selected() ? "取消选择" : "加入选择"}</button><button data-open-detail>查看详情</button>`;
  menu.innerHTML = linkChoices;
  const canvasRect = document
    .querySelector("#canvasPage")
    .getBoundingClientRect();
  menu.style.left = `${Math.min(canvasRect.width - 190, Math.max(8, renderedPosition.x + 12))}px`;
  menu.style.top = `${Math.min(canvasRect.height - 230, Math.max(8, renderedPosition.y + 12))}px`;
  document.querySelector("#canvasPage").appendChild(menu);
  menu.querySelector("[data-start-link]")?.addEventListener("click", () => {
    quickLinkSource = id;
    cy.nodes().removeClass("link-source");
    node.addClass("link-source");
    closeTopologyMenu();
    toast(
      `已选择“${nameOf(devices.find((device) => device.id === id))}”作为上级，请右键或长按目标设备`,
    );
  });
  menu.querySelector("[data-toggle-select]")?.addEventListener("click", () => {
    node.selected() ? node.unselect() : node.select();
    closeTopologyMenu();
  });
  menu.querySelector("[data-open-detail]")?.addEventListener("click", () => {
    closeTopologyMenu();
    openDetail(id);
  });
  menu.querySelector("[data-cancel-link]")?.addEventListener("click", () => {
    quickLinkSource = null;
    cy.nodes().removeClass("link-source");
    closeTopologyMenu();
  });
  menu
    .querySelectorAll("[data-link-type]")
    .forEach((button) =>
      button.addEventListener("click", () =>
        createQuickConnection(id, button.dataset.linkType),
      ),
    );
}

function openConnectionMenu(edge, renderedPosition) {
  closeTopologyMenu();
  const targetID = +(edge.data("target") || 0);
  const connectionID = +(String(edge.id()).replace(/^e/, "") || 0);
  const source = devices.find(
    (device) => device.id === +(edge.data("source") || 0),
  );
  const target = devices.find((device) => device.id === targetID);
  const menu = document.createElement("div");
  menu.id = "topologyMenu";
  menu.className = "topology-menu";
  const sourceType = edge.data("sourceType") || "inferred";
  const confidence = Number(edge.data("confidence") || 0);
  menu.innerHTML = `<div class="topology-menu-title">${esc(source ? nameOf(source) : "上级设备")} → ${esc(target ? nameOf(target) : "目标设备")}</div><small>${esc(connectionTypeLabel(edge.data("type")))} · ${esc(connectionSourceLabel(sourceType))}${confidence ? ` · ${Math.round(confidence * 100)}%` : ""}</small>${can(permission.edit) ? "<button data-remove-edge>移除这条连接</button>" : ""}`;
  const canvasRect = document
    .querySelector("#canvasPage")
    .getBoundingClientRect();
  menu.style.left = `${Math.min(canvasRect.width - 210, Math.max(8, renderedPosition.x + 12))}px`;
  menu.style.top = `${Math.min(canvasRect.height - 150, Math.max(8, renderedPosition.y + 12))}px`;
  document.querySelector("#canvasPage").appendChild(menu);
  menu
    .querySelector("[data-remove-edge]")
    ?.addEventListener("click", async () => {
      if (!confirm("确定移除这条连接吗？设备本身不会被删除。")) return;
      try {
        await api(`/api/devices/${targetID}/connections/${connectionID}`, {
          method: "DELETE",
        });
        closeTopologyMenu();
        await refresh();
        toast("连接已移除");
      } catch (error) {
        toast(error.message);
      }
    });
}

appExtensions.topologyReady = function () {
  if (!cy) return;
  cy.boxSelectionEnabled(false);
  cy.selectionType("additive");
  cy.style()
    .selector("node:selected")
    .style({
      "border-width": 6,
      "border-color": "#35c98a",
      "border-opacity": 1,
      "underlay-color": "#35c98a",
      "underlay-opacity": 0.34,
      "underlay-padding": 15,
      opacity: 1,
      "z-index": 20,
    })
    .selector("node.link-source")
    .style({
      "border-width": 6,
      "border-color": "#df9a3d",
      "underlay-color": "#df9a3d",
      "underlay-opacity": 0.3,
      "underlay-padding": 15,
      opacity: 1,
      "z-index": 21,
    })
    .update();
  cy.off("tap", "node");
  cy.on("tap", "node", (event) => {
    if (mobileSelectionMode) {
      event.target.selected() ? event.target.unselect() : event.target.select();
      return;
    }
    openDetail(+event.target.id());
  });
  cy.off("dragfree", "node");
  cy.on("dragfree", "node", (event) => {
    const moved = event.target.selected()
      ? cy.nodes(":selected")
      : event.target;
    moved.forEach((node) => savePosition(node));
  });
  cy.on("cxttap", "node", (event) =>
    openTopologyMenu(event.target, event.renderedPosition),
  );
  cy.on("cxttap", "edge", (event) =>
    openConnectionMenu(event.target, event.renderedPosition),
  );
  cy.on("select unselect", "node", updateSelectionBar);
  cy.on("tap pan zoom", (event) => {
    if (event.target === cy) closeTopologyMenu();
  });
  document
    .querySelector("#cy")
    .addEventListener("contextmenu", (event) => event.preventDefault());
  installDirectBoxSelection();
};
