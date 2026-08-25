/* Cytoscape rendering, layout and topology state. */
function initCy() {
  if (!window.cytoscape) {
    $("#cy").innerHTML =
      '<div style="padding:3rem;text-align:center">拓扑组件未加载，请检查本地静态资源。</div>';
    return;
  }
  cy = cytoscape({
    container: $("#cy"),
    elements: [],
    style: [
      {
        selector: "node",
        style: {
          shape: "roundrectangle",
          width: 154,
          height: 68,
          "background-color": "#fffdf9",
          "border-width": 2,
          "border-color": "#b8c8c1",
          label: "data(display)",
          "text-valign": "center",
          "text-halign": "center",
          "font-size": 12,
          "font-weight": 600,
          "line-height": 1.5,
          color: "#314941",
          "text-wrap": "wrap",
          "text-max-width": 138,
          "overlay-opacity": 0,
        },
      },
      {
        selector: 'node[status="online"]',
        style: { "background-color": "#effaf4", "border-color": "#57bd82" },
      },
      {
        selector: 'node[status="suspected_offline"]',
        style: { "background-color": "#fff8e5", "border-color": "#dfb23f" },
      },
      {
        selector: 'node[status="offline"]',
        style: {
          "background-color": "#f0f3f2",
          "border-color": "#9ca9a4",
          color: "#78847f",
          opacity: 0.72,
        },
      },
      {
        selector: 'node[status="unknown"]',
        style: { "background-color": "#f6f7f6", "border-color": "#aab7b2" },
      },
      {
        selector: 'node[isnew="true"]',
        style: {
          "underlay-color": "#eea35b",
          "underlay-opacity": 0.16,
          "underlay-padding": 7,
          "underlay-shape": "roundrectangle",
        },
      },
      {
        selector: 'node[type="gateway"],node[type="router"]',
        style: {
          width: 176,
          height: 76,
          "background-color": "#dcf4e9",
          "border-width": 3,
          "border-color": "#45ad75",
          "font-size": 13,
        },
      },
      {
        selector: 'node[type="internet"]',
        style: {
          width: 160,
          height: 72,
          "background-color": "#eaf3fb",
          "border-color": "#78a9ca",
          "font-size": 13,
        },
      },
      {
        selector: "node:selected",
        style: {
          "border-width": 4,
          "border-color": "#2c9b68",
          "underlay-color": "#78c7a8",
          "underlay-opacity": 0.2,
          "underlay-padding": 9,
        },
      },
      {
        selector: "edge",
        style: {
          "curve-style": "taxi",
          "taxi-direction": "downward",
          "taxi-turn": 22,
          "taxi-turn-min-distance": 12,
          width: 2,
          "line-color": "#9fb4ab",
          "target-arrow-shape": "none",
          opacity: 0.7,
        },
      },
      {
        selector: 'edge[sourceType="inferred"]',
        style: {
          "line-style": "dashed",
          "line-color": "#c1cec8",
          opacity: 0.65,
        },
      },
      {
        selector: 'edge[type="wifi"]',
        style: { "line-style": "dashed", "line-color": "#79aeca" },
      },
      { selector: 'edge[type="unknown"]', style: { "line-style": "dotted" } },
      {
        selector: 'edge[confirmed="true"]',
        style: {
          "line-style": "solid",
          "line-color": "#6f9d89",
          opacity: 0.95,
          width: 3,
        },
      },
    ],
    layout: { name: "grid", padding: 55 },
  });
  cy.on("tap", "node", (e) => openDetail(+e.target.id()));
  cy.on("dragfree", "node", (e) => savePosition(e.target));
  appExtensions.topologyReady();
}
function applyTopologyTheme() {
  if (!cy) return;
  const dark = document.documentElement.classList.contains("dark");
  const colors = dark
    ? {
        base: "#24342d",
        text: "#e8f1ed",
        border: "#465e54",
        online: "#223d32",
        onlineBorder: "#62c78c",
        warn: "#443a22",
        warnBorder: "#d8ae45",
        off: "#293530",
        offBorder: "#71847b",
        muted: "#a3b6ae",
        gateway: "#24483a",
        gatewayBorder: "#64c692",
        internet: "#243b49",
        internetBorder: "#6fa5c7",
      }
    : {
        base: "#fffdf9",
        text: "#314941",
        border: "#b8c8c1",
        online: "#effaf4",
        onlineBorder: "#57bd82",
        warn: "#fff8e5",
        warnBorder: "#dfb23f",
        off: "#f0f3f2",
        offBorder: "#9ca9a4",
        muted: "#78847f",
        gateway: "#dcf4e9",
        gatewayBorder: "#45ad75",
        internet: "#eaf3fb",
        internetBorder: "#78a9ca",
      };
  cy.style()
    .selector("node")
    .style({
      "background-color": colors.base,
      "border-color": colors.border,
      color: colors.text,
    })
    .selector('node[status="online"]')
    .style({
      "background-color": colors.online,
      "border-color": colors.onlineBorder,
      color: colors.text,
    })
    .selector('node[status="suspected_offline"]')
    .style({
      "background-color": colors.warn,
      "border-color": colors.warnBorder,
      color: colors.text,
    })
    .selector('node[status="offline"]')
    .style({
      "background-color": colors.off,
      "border-color": colors.offBorder,
      color: colors.muted,
    })
    .selector('node[type="gateway"],node[type="router"]')
    .style({
      "background-color": colors.gateway,
      "border-color": colors.gatewayBorder,
    })
    .selector('node[type="internet"]')
    .style({
      "background-color": colors.internet,
      "border-color": colors.internetBorder,
    })
    .update();
}
const iconDrawings = {
  internet:
    '<path d="M7 18h10a4 4 0 0 0 .4-8A6 6 0 0 0 6 8.5 4.8 4.8 0 0 0 7 18Z"/><path d="M8 21h8"/>',
  modem:
    '<rect x="4" y="7" width="16" height="11" rx="3"/><path d="M8 12h.01M12 12h.01M16 12h.01M8 21h8"/>',
  gateway:
    '<rect x="3" y="9" width="18" height="9" rx="3"/><path d="M7 13h.01M11 13h.01M17 13h.01M12 9V5m-4 2 4-2 4 2"/>',
  router:
    '<rect x="3" y="9" width="18" height="9" rx="3"/><path d="M7 13h.01M11 13h.01M17 13h.01M12 9V5m-4 2 4-2 4 2"/>',
  switch:
    '<rect x="3" y="6" width="18" height="12" rx="3"/><path d="M7 10h2m2 0h2m2 0h2M7 14h2m2 0h2m2 0h2M8 21h8"/>',
  ap: '<path d="M5 10a10 10 0 0 1 14 0M8 13a6 6 0 0 1 8 0m-5 3a2 2 0 0 1 2 0"/><circle cx="12" cy="19" r="1"/>',
  nas: '<rect x="4" y="3" width="16" height="8" rx="2"/><rect x="4" y="13" width="16" height="8" rx="2"/><path d="M8 7h.01M8 17h.01M12 7h5M12 17h5"/>',
  linux:
    '<rect x="3" y="4" width="18" height="16" rx="3"/><path d="m7 9 3 3-3 3m5 0h5"/>',
  windows:
    '<path d="M4 5.5 11 4v7H4Zm9-1.8 7-1.2V11h-7ZM4 13h7v7l-7-1.5Zm9 0h7v8.5L13 20Z"/>',
  macos:
    '<rect x="3" y="4" width="18" height="14" rx="2"/><path d="M8 21h8m-4-3v3"/>',
  phone:
    '<rect x="7" y="2" width="10" height="20" rx="3"/><path d="M11 18h2"/>',
  tablet:
    '<rect x="5" y="2" width="14" height="20" rx="3"/><path d="M11 18h2"/>',
  tv: '<rect x="2" y="4" width="20" height="14" rx="3"/><path d="m8 22 4-4 4 4"/>',
  camera:
    '<rect x="3" y="7" width="13" height="10" rx="3"/><path d="m16 10 5-3v10l-5-3Z"/><circle cx="9" cy="12" r="2"/>',
  iot: '<path d="M9 18h6m-5 3h4M8 14a6 6 0 1 1 8 0c-1 1-1 2-1 3H9c0-1 0-2-1-3Z"/>',
  game: '<path d="M8 8h8a5 5 0 0 1 4.5 3l1.2 3.5a3 3 0 0 1-5 3L15 16H9l-1.7 1.5a3 3 0 0 1-5-3L3.5 11A5 5 0 0 1 8 8Z"/><path d="M7 11v4m-2-2h4m7-1h.01m2 2h.01"/>',
  printer:
    '<path d="M6 9V3h12v6M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2"/><rect x="6" y="14" width="12" height="8" rx="1"/>',
  room: '<path d="m3 11 9-8 9 8v10H6V11"/><path d="M10 21v-6h4v6"/>',
  unknown:
    '<circle cx="12" cy="12" r="9"/><path d="M9.8 9a2.5 2.5 0 1 1 3.5 2.3c-.9.4-1.3 1-1.3 2.2M12 17h.01"/>',
};
function deviceIcon(type, color) {
  const body = iconDrawings[type] || iconDrawings.unknown;
  return `data:image/svg+xml;utf8,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 24 24" preserveAspectRatio="xMidYMid meet" fill="none" stroke="${color}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">${body}</svg>`)}`;
}
function deviceArtwork(type, dark = false) {
  const files = {
    modem: "topo-router.png",
    gateway: "topo-router.png",
    router: "topo-router.png",
    ap: "topo-router.png",
    switch: "topo-switch.png",
    nas: "topo-nas.png",
    linux: "topo-computer.png",
    windows: "topo-computer.png",
    macos: "topo-computer.png",
    phone: "topo-phone.png",
    tablet: "topo-phone.png",
    tv: "topo-tv.png",
    iot: "topo-iot.png",
    game: "topo-game.png",
    printer: "topo-printer.png",
    unknown: "topo-unknown.png",
  };
  return files[type]
    ? `/assets/devices/${files[type]}`
    : deviceIcon(type, dark ? "#9fe0c2" : "#376252");
}
function applyIconStyle() {
  if (!cy) return;
  const dark = document.documentElement.classList.contains("dark"),
    canvasBg = dark ? "#17241f" : "#f8f6f1",
    labelBg = dark ? "#202f29" : "#fffdf9",
    labelBorder = dark ? "#40584e" : "#d5e3dd";
  cy.style()
    .selector("node")
    .style({
      shape: "ellipse",
      width: 116,
      height: 116,
      "background-color": canvasBg,
      "border-width": 0,
      "background-image": "data(art)",
      "background-image-opacity": 1,
      "background-width": "auto",
      "background-height": "auto",
      "background-position-x": "50%",
      "background-position-y": "42%",
      "background-fit": "contain",
      "background-clip": "none",
      "text-valign": "bottom",
      "text-halign": "center",
      "text-margin-x": 0,
      "text-margin-y": 22,
      "text-max-width": 132,
      "text-background-color": labelBg,
      "text-background-opacity": 0.94,
      "text-background-shape": "roundrectangle",
      "text-background-padding": 6,
      "text-border-color": labelBorder,
      "text-border-width": 1,
      "text-border-opacity": 1,
      "underlay-shape": "ellipse",
      "underlay-padding": 5,
    })
    .selector('node[status="online"]')
    .style({
      "background-color": canvasBg,
      "border-width": 0,
      "underlay-color": "#62c78c",
      "underlay-opacity": 0.11,
    })
    .selector('node[status="suspected_offline"]')
    .style({
      "background-color": canvasBg,
      "border-width": 0,
      "underlay-color": "#d8ae45",
      "underlay-opacity": 0.14,
    })
    .selector('node[status="offline"]')
    .style({
      "background-color": canvasBg,
      "border-width": 0,
      "underlay-color": "#83978d",
      "underlay-opacity": 0.08,
    })
    .selector('node[status="unknown"]')
    .style({
      "background-color": canvasBg,
      "border-width": 0,
      "underlay-color": "#9caea6",
      "underlay-opacity": 0.08,
    })
    .selector('node[isnew="true"]')
    .style({
      "underlay-color": "#eea35b",
      "underlay-opacity": 0.2,
      "underlay-padding": 8,
    })
    .selector('node[type="gateway"],node[type="router"],node[type="internet"]')
    .style({
      width: 128,
      height: 128,
      "background-color": canvasBg,
      "border-width": 0,
    })
    .selector("node.found")
    .style({
      "underlay-color": "#3aa7d6",
      "underlay-opacity": 0.5,
      "underlay-padding": 9,
    })
    .update();
}
function nodeDisplay(d) {
  const name = nameOf(d),
    type = typeOf(d),
    ip = d.current_ip || "";
  const label = name === ip
    ? `${typeLabel(type)}\n${statusText(d.status)}`
    : `${name}\n${ip || typeLabel(type)} · ${statusText(d.status)}`;
  return `${appExtensions.deviceLabel(d)}${label}`;
}
function elements() {
  const dark = document.documentElement.classList.contains("dark");
  const nodes = devices
    .filter((d) => !d.is_hidden)
    .map((d) => {
      const type = typeOf(d);
      return {
        data: {
          id: String(d.id),
          display: nodeDisplay(d),
          art: deviceArtwork(type, dark),
          status: d.status,
          isnew: String(d.is_new),
          type,
        },
        position: d.x || d.y ? { x: d.x, y: d.y } : undefined,
        locked: d.locked,
      };
    });
  const ids = new Set(nodes.map((n) => n.data.id));
  const edges = connections
    .filter(
      (c) =>
        ids.has(String(c.source_device_id)) &&
        ids.has(String(c.target_device_id)),
    )
    .map((c) => ({
      data: {
        id: `e${c.id}`,
        source: String(c.source_device_id),
        target: String(c.target_device_id),
        type: c.connection_type,
        sourceType: c.source_type,
        confidence: c.confidence ?? 0,
        confirmed: String(c.user_confirmed),
      },
    }));
  return [...nodes, ...edges];
}
function runTopologyLayout(animate = false) {
  if (!cy || !cy.nodes().length) return;
  const roots = cy.nodes('[type="internet"]');
  const layout = cy.layout({
    name: "breadthfirst",
    directed: true,
    roots: roots.length ? roots : undefined,
    grid: true,
    spacingFactor: 1,
    padding: 60,
    animate,
    animationDuration: 360,
    avoidOverlap: true,
    nodeDimensionsIncludeLabels: true,
  });
  layout.on("layoutstop", () => {
    cy.nodes()
      .filter((n) => !n.locked())
      .forEach((n) => savePosition(n));
    localStorage.meowtopoLayout = "artwork-v1";
  });
  layout.run();
}
async function refresh(runLayout = false) {
  const t = await api("/api/topology");
  devices = t.devices || [];
  connections = t.connections || [];
  updateStats();
  if (cy) {
    const had = cy.nodes().length;
    cy.elements().remove();
    cy.add(elements());
    const upgradeLayout = localStorage.meowtopoLayout !== "artwork-v1";
    if (
      runLayout ||
      upgradeLayout ||
      !had ||
      devices.every((d) => !d.x && !d.y)
    )
      runTopologyLayout(false);
    else
      devices
        .filter((d) => d.locked)
        .forEach((d) => cy.$id(String(d.id)).lock());
  }
  $("#empty").classList.toggle("hidden", devices.length > 0);
  appExtensions.refreshed();
}
function updateStats() {
  const c = { online: 0, suspected_offline: 0, offline: 0 };
  devices.forEach((d) => (c[d.status] = (c[d.status] || 0) + 1));
  $("#onlineN").textContent = c.online || 0;
  $("#suspectN").textContent = c.suspected_offline || 0;
  $("#offlineN").textContent = c.offline || 0;
  $("#totalN").textContent = devices.length;
}
function savePosition(n) {
  clearTimeout(saveTimers.get(n.id()));
  saveTimers.set(
    n.id(),
    setTimeout(
      () =>
        api(`/api/devices/${n.id()}/position`, {
          method: "PATCH",
          body: JSON.stringify({
            x: n.position("x"),
            y: n.position("y"),
            locked: n.locked(),
          }),
        }).catch((e) => toast(e.message)),
      350,
    ),
  );
}
