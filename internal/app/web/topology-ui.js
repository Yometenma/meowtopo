/* Topology canvas interactions and the workspace's dynamic bindings. */
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
