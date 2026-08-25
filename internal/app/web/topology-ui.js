/* Topology canvas interactions and the workspace's dynamic bindings. */
let linkingChild = null;
let linkArrow = null;
let lastLinkMouse = null;
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
      const preview = (currentX, currentY) => {
        const left = Math.min(x, currentX);
        const right = Math.max(x, currentX);
        const top = Math.min(y, currentY);
        const bottom = Math.max(y, currentY);
        cy.nodes(":visible").forEach((node) => {
          const position = node.renderedPosition();
          const inside =
            position.x >= left &&
            position.x <= right &&
            position.y >= top &&
            position.y <= bottom;
          node.toggleClass("box-preview", inside);
        });
      };
      const move = (moveEvent) => {
        const currentX = moveEvent.clientX - pageRect.left;
        const currentY = moveEvent.clientY - pageRect.top;
        marquee.style.left = `${Math.min(startX, currentX)}px`;
        marquee.style.top = `${Math.min(startY, currentY)}px`;
        marquee.style.width = `${Math.abs(currentX - startX)}px`;
        marquee.style.height = `${Math.abs(currentY - startY)}px`;
        preview(
          moveEvent.clientX - containerRect.left,
          moveEvent.clientY - containerRect.top,
        );
      };
      const finish = (upEvent) => {
        window.removeEventListener("mousemove", move, true);
        window.removeEventListener("mouseup", finish, true);
        const endX = upEvent.clientX - containerRect.left;
        const endY = upEvent.clientY - containerRect.top;
        cy.nodes(":visible").removeClass("box-preview");
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

function cancelParentLink() {
  linkingChild = null;
  if (linkArrow) {
    linkArrow.svg.remove();
    linkArrow = null;
  }
  lastLinkMouse = null;
  cy.nodes().removeClass("link-source");
}

function startParentLink(childId) {
  cancelParentLink();
  linkingChild = childId;
  const accent = cssVar("--acc", "#3f7a55");
  cy.$id(String(childId)).addClass("link-source");
  const container = document.querySelector("#cy");
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.style.cssText =
    "position:absolute;inset:0;width:100%;height:100%;pointer-events:none;z-index:15;";
  const defs = document.createElementNS("http://www.w3.org/2000/svg", "defs");
  const marker = document.createElementNS(
    "http://www.w3.org/2000/svg",
    "marker",
  );
  marker.setAttribute("id", "linkFollowArrow");
  marker.setAttribute("markerWidth", "10");
  marker.setAttribute("markerHeight", "10");
  marker.setAttribute("refX", "8");
  marker.setAttribute("refY", "3");
  marker.setAttribute("orient", "auto");
  marker.setAttribute("markerUnits", "strokeWidth");
  const arrowPath = document.createElementNS(
    "http://www.w3.org/2000/svg",
    "path",
  );
  arrowPath.setAttribute("d", "M0,0 L0,6 L9,3 z");
  arrowPath.setAttribute("fill", accent);
  marker.appendChild(arrowPath);
  defs.appendChild(marker);
  const line = document.createElementNS("http://www.w3.org/2000/svg", "line");
  line.setAttribute("stroke", accent);
  line.setAttribute("stroke-width", "2");
  line.setAttribute("stroke-dasharray", "6 4");
  line.setAttribute("marker-end", "url(#linkFollowArrow)");
  svg.appendChild(defs);
  svg.appendChild(line);
  container.appendChild(svg);
  linkArrow = { svg, line };
  const childDevice = devices.find((device) => device.id === childId);
  toast(`已选择“${nameOf(childDevice)}”，请点击要设为其父级的设备`);
  updateLinkArrow();
}

function updateLinkArrow() {
  if (!linkArrow || !cy || !linkingChild) return;
  const childNode = cy.$id(String(linkingChild));
  if (!childNode.length || !lastLinkMouse) return;
  const pos = childNode.renderedPosition();
  linkArrow.line.setAttribute("x1", pos.x);
  linkArrow.line.setAttribute("y1", pos.y);
  linkArrow.line.setAttribute("x2", lastLinkMouse.x);
  linkArrow.line.setAttribute("y2", lastLinkMouse.y);
}

function recordLinkMouse(event) {
  const rect = document.querySelector("#cy").getBoundingClientRect();
  lastLinkMouse = { x: event.clientX - rect.left, y: event.clientY - rect.top };
  updateLinkArrow();
}

function finishParentLink(parentId, renderedPosition) {
  const childId = linkingChild;
  cancelParentLink();
  const parent = devices.find((device) => device.id === parentId);
  const menu = document.createElement("div");
  menu.id = "topologyMenu";
  menu.className = "topology-menu";
  menu.innerHTML = `<div class="topology-menu-title">将 ${esc(nameOf(parent))} 设为父级</div><button data-link-type="ethernet">网线连接</button><button data-link-type="wifi">Wi-Fi 连接</button><button data-link-type="logical">逻辑连接</button><button data-link-type="virtual">虚拟连接</button><button data-cancel-link>取消</button>`;
  const canvasRect = document
    .querySelector("#canvasPage")
    .getBoundingClientRect();
  menu.style.left = `${Math.min(canvasRect.width - 190, Math.max(8, renderedPosition.x + 12))}px`;
  menu.style.top = `${Math.min(canvasRect.height - 230, Math.max(8, renderedPosition.y + 12))}px`;
  document.querySelector("#canvasPage").appendChild(menu);
  menu.querySelectorAll("[data-link-type]").forEach((button) =>
    button.addEventListener("click", () =>
      createParentConnection(childId, parentId, button.dataset.linkType),
    ),
  );
  menu
    .querySelector("[data-cancel-link]")
    .addEventListener("click", closeTopologyMenu);
}

async function createParentConnection(childId, parentId, connectionType) {
  try {
    await api(`/api/devices/${childId}/connections`, {
      method: "POST",
      body: JSON.stringify({
        parent_id: parentId,
        connection_type: connectionType,
        port_label: "",
      }),
    });
    closeTopologyMenu();
    await refresh();
    const parent = devices.find((device) => device.id === parentId);
    const child = devices.find((device) => device.id === childId);
    toast(`已连接：${nameOf(parent)} → ${nameOf(child)}`);
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
  menu.innerHTML = `${editable ? "<button data-set-parent>设置父级</button>" : ""}<button data-toggle-select>${node.selected() ? "取消选择" : "加入选择"}</button><button data-open-detail>查看详情</button>`;
  const canvasRect = document
    .querySelector("#canvasPage")
    .getBoundingClientRect();
  menu.style.left = `${Math.min(canvasRect.width - 190, Math.max(8, renderedPosition.x + 12))}px`;
  menu.style.top = `${Math.min(canvasRect.height - 230, Math.max(8, renderedPosition.y + 12))}px`;
  document.querySelector("#canvasPage").appendChild(menu);
  menu.querySelector("[data-set-parent]")?.addEventListener("click", () => {
    closeTopologyMenu();
    startParentLink(id);
  });
  menu.querySelector("[data-toggle-select]")?.addEventListener("click", () => {
    node.selected() ? node.unselect() : node.select();
    closeTopologyMenu();
  });
  menu.querySelector("[data-open-detail]")?.addEventListener("click", () => {
    closeTopologyMenu();
    openDetail(id);
  });
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
  const accent = cssVar("--acc", "#8fe3b8");
  const fresh = cssVar("--new", "#e8bd9a");
  cy.boxSelectionEnabled(false);
  cy.selectionType("additive");
  cy.style()
    .selector("node:selected")
    .style({
      "border-width": 6,
      "border-color": accent,
      "border-opacity": 1,
      "underlay-color": accent,
      "underlay-opacity": 0.5,
      "underlay-padding": 16,
      opacity: 1,
      "z-index": 20,
    })
    .selector("node.box-preview")
    .style({
      "border-width": 4,
      "border-color": accent,
      "underlay-color": accent,
      "underlay-opacity": 0.32,
      "underlay-padding": 12,
      "z-index": 19,
    })
    .selector("node.link-source")
    .style({
      "border-width": 5,
      "border-color": fresh,
      "underlay-color": fresh,
      "underlay-opacity": 0.3,
      "underlay-padding": 14,
      opacity: 1,
      "z-index": 21,
    })
    .update();
  cy.off("tap", "node");
  cy.on("tap", "node", (event) => {
    const id = +event.target.id();
    if (linkingChild) {
      if (id !== linkingChild) finishParentLink(id, event.renderedPosition);
      return;
    }
    if (mobileSelectionMode) {
      event.target.selected() ? event.target.unselect() : event.target.select();
      return;
    }
    openDetail(id);
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
  cy.on("tap", (event) => {
    if (event.target !== cy) return;
    closeTopologyMenu();
    document.querySelector("#drawer").classList.remove("open");
    if (linkingChild) cancelParentLink();
  });
  cy.on("pan zoom", () => {
    closeTopologyMenu();
    updateLinkArrow();
  });
  document
    .querySelector("#cy")
    .addEventListener("contextmenu", (event) => event.preventDefault());
  document.addEventListener("mousemove", (event) => {
    if (linkArrow) recordLinkMouse(event);
  });
  installDirectBoxSelection();
};
