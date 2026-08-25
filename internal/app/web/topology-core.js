/* Cytoscape rendering, layout and topology state. */
function cssVar(name, fallback) {
  const value = getComputedStyle(document.documentElement)
    .getPropertyValue(name)
    .trim();
  // Cytoscape's color parser rejects 8-digit hex alpha; opacity props
  // already carry the alpha, so normalize to 6-digit hex.
  if (/^#[0-9a-f]{8}$/i.test(value)) return value.slice(0, 7);
  return value || fallback;
}
function themeColors() {
  return {
    win: cssVar("--win", "#f7f9f7"),
    chip: cssVar("--chip", "#f0f3f0"),
    panel: cssVar("--panel-solid", "#ffffff"),
    text: cssVar("--text", "#2e3338"),
    muted: cssVar("--muted", "#828c88"),
    line: cssVar("--line2", "#2a2f353a"),
    acc: cssVar("--acc", "#3f7a55"),
    ok: cssVar("--ok", "#3f7a55"),
    warn: cssVar("--warn", "#96702a"),
    bad: cssVar("--bad", "#a84a4f"),
    info: cssVar("--info", "#3f7a8c"),
    fresh: cssVar("--new", "#a8763d"),
  };
}
function initCy() {
  if (!window.cytoscape) {
    $("#cy").innerHTML =
      '<div style="padding:3rem;text-align:center">拓扑组件未加载，请检查本地静态资源。</div>';
    return;
  }
  const c = themeColors();
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
          "background-color": c.chip,
          "border-width": 1.5,
          "border-color": c.line,
          label: "data(display)",
          "text-valign": "center",
          "text-halign": "center",
          "font-size": 12,
          "font-weight": 600,
          "line-height": 1.5,
          color: c.text,
          "text-wrap": "wrap",
          "text-max-width": 138,
          "overlay-opacity": 0,
        },
      },
      {
        selector: 'node[status="online"]',
        style: { "border-color": c.ok },
      },
      {
        selector: 'node[status="suspected_offline"]',
        style: { "border-color": c.warn },
      },
      {
        selector: 'node[status="offline"]',
        style: { "border-color": c.muted, color: c.muted, opacity: 0.6 },
      },
      {
        selector: 'node[status="unknown"]',
        style: { "border-color": c.muted },
      },
      {
        selector: 'node[isnew="true"]',
        style: {
          "underlay-color": c.fresh,
          "underlay-opacity": 0.18,
          "underlay-padding": 7,
          "underlay-shape": "roundrectangle",
        },
      },
      {
        selector: 'node[type="gateway"],node[type="router"]',
        style: { width: 176, height: 76, "border-width": 2, "font-size": 13 },
      },
      {
        selector: 'node[type="internet"]',
        style: { width: 160, height: 72, "border-color": c.info },
      },
      {
        selector: "node:selected",
        style: {
          "border-width": 3,
          "border-color": c.acc,
          "underlay-color": c.acc,
          "underlay-opacity": 0.16,
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
          width: 1.6,
          "line-color": c.muted,
          "target-arrow-shape": "none",
          opacity: 0.55,
        },
      },
      {
        selector: 'edge[sourceType="inferred"]',
        style: { "line-style": "dashed", opacity: 0.35 },
      },
      {
        selector: 'edge[type="wifi"]',
        style: { "line-style": "dashed", "line-color": c.info },
      },
      { selector: 'edge[type="unknown"]', style: { "line-style": "dotted" } },
      {
        selector: 'edge[confirmed="true"]',
        style: {
          "line-style": "solid",
          "line-color": c.acc,
          opacity: 0.85,
          width: 2.4,
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
  const c = themeColors();
  cy.style()
    .selector("node")
    .style({
      "background-color": c.chip,
      "border-color": c.line,
      color: c.text,
    })
    .selector('node[status="online"]')
    .style({ "border-color": c.ok, color: c.text })
    .selector('node[status="suspected_offline"]')
    .style({ "border-color": c.warn, color: c.text })
    .selector('node[status="offline"]')
    .style({ "border-color": c.muted, color: c.muted })
    .selector('node[status="unknown"]')
    .style({ "border-color": c.muted })
    .selector('node[type="internet"]')
    .style({ "border-color": c.info })
    .selector("edge")
    .style({ "line-color": c.muted })
    .selector('edge[type="wifi"]')
    .style({ "line-color": c.info })
    .selector('edge[confirmed="true"]')
    .style({ "line-color": c.acc })
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
    : deviceIcon(type, cssVar("--acc", dark ? "#8fe3b8" : "#3f7a55"));
}
function applyIconStyle() {
  if (!cy) return;
  const c = themeColors();
  cy.style()
    .selector("node")
    .style({
      shape: "ellipse",
      width: 112,
      height: 112,
      "background-color": c.win,
      "border-width": 2.5,
      "border-color": c.line,
      "background-image": "data(art)",
      "background-image-opacity": 1,
      "background-width": "auto",
      "background-height": "auto",
      "background-position-x": "50%",
      "background-position-y": "42%",
      "background-fit": "contain",
      "background-clip": "none",
      "font-family": cssVar("--mono", "monospace"),
      "font-size": 10.5,
      "font-weight": 600,
      "text-valign": "bottom",
      "text-halign": "center",
      "text-margin-x": 0,
      "text-margin-y": 20,
      "text-max-width": 132,
      "text-background-color": c.panel,
      "text-background-opacity": 0.94,
      "text-background-shape": "roundrectangle",
      "text-background-padding": 6,
      "text-border-color": c.line,
      "text-border-width": 1,
      "text-border-opacity": 1,
      "underlay-shape": "ellipse",
      "underlay-padding": 6,
    })
    .selector('node[status="online"]')
    .style({
      "border-color": c.ok,
      "underlay-color": c.ok,
      "underlay-opacity": 0.2,
    })
    .selector('node[status="suspected_offline"]')
    .style({
      "border-color": c.warn,
      "underlay-color": c.warn,
      "underlay-opacity": 0.22,
    })
    .selector('node[status="offline"]')
    .style({
      "border-color": c.muted,
      "underlay-color": c.muted,
      "underlay-opacity": 0.1,
      opacity: 0.55,
    })
    .selector('node[status="unknown"]')
    .style({
      "border-color": c.line,
      "underlay-color": c.muted,
      "underlay-opacity": 0.08,
    })
    .selector('node[isnew="true"]')
    .style({
      "underlay-color": c.fresh,
      "underlay-opacity": 0.26,
      "underlay-padding": 8,
    })
    .selector('node[important="true"]')
    .style({
      width: 128,
      height: 128,
      "font-size": 11,
    })
    .selector('node[type="gateway"],node[type="router"],node[type="internet"]')
    .style({
      width: 128,
      height: 128,
    })
    .selector('node[type="internet"]')
    .style({
      "border-color": c.info,
    })
    .selector("node.found")
    .style({
      "underlay-color": c.info,
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
          important: String(!!d.is_important),
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
function runTopologyLayout(animate = false, fitAfter = false) {
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
    if (fitAfter) cy.fit(undefined, 60);
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
    const freshLayout =
      !had || devices.every((d) => !d.x && !d.y);
    if (runLayout || upgradeLayout || freshLayout)
      runTopologyLayout(false, freshLayout);
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
