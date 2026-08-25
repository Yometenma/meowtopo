/* Device manager and activity history core. */
function openManager() {
  if ($("#managerType").options.length === 1)
    $("#managerType").insertAdjacentHTML(
      "beforeend",
      Object.keys(icons)
        .filter((v) => v !== "camera")
        .map((v) => `<option value="${v}">${typeLabel(v)}</option>`)
        .join(""),
    );
  selectedDevices.clear();
  $("#batchParent").innerHTML =
    '<option value="">选择父设备…</option>' +
    devices
      .map((d) => `<option value="${d.id}">${esc(nameOf(d))}</option>`)
      .join("");
  renderManager();
  appExtensions.managerOpened();
}
function renderManager() {
  const list = $("#managerList");
  if (!list) return;
  const query = $("#managerSearch").value.trim().toLowerCase();
  const status = $("#managerStatus").value;
  const type = $("#managerType").value;
  const visibility = $("#managerVisibility").value;
  managerFiltered = devices.filter(
    (d) =>
      (!query ||
        [nameOf(d), d.current_ip, d.mac_address, d.auto_hostname].some((v) =>
          (v || "").toLowerCase().includes(query),
        )) &&
      (!status || d.status === status) &&
      (!type || typeOf(d) === type) &&
      (!visibility || (visibility === "hidden") === !!d.is_hidden),
  );
  const validIDs = new Set(devices.map((d) => d.id));
  selectedDevices.forEach((id) => {
    if (!validIDs.has(id)) selectedDevices.delete(id);
  });
  $("#managerSummary").textContent =
    `显示 ${managerFiltered.length} / ${devices.length} 台设备 · 已隐藏 ${devices.filter((d) => d.is_hidden).length} 台`;
  $("#managerSelected").textContent = `已选 ${selectedDevices.size} 台`;
  const all = $("#managerSelectAll");
  all.checked =
    managerFiltered.length > 0 &&
    managerFiltered.every((d) => selectedDevices.has(d.id));
  all.indeterminate =
    !all.checked && managerFiltered.some((d) => selectedDevices.has(d.id));
  list.innerHTML = managerFiltered.length
    ? managerFiltered
        .map(
          (d) =>
            `<div class="manager-device"><div class="manager-device-main"><input type="checkbox" data-select="${d.id}" aria-label="选择 ${attr(nameOf(d))}" ${selectedDevices.has(d.id) ? "checked" : ""}><div class="device-icon"><img src="${deviceArtwork(typeOf(d))}" alt="" loading="lazy"></div><div><b>${esc(nameOf(d))}${d.is_new ? '<span class="badge warn">新设备</span>' : ""}${d.is_hidden ? '<span class="badge">已隐藏</span>' : ""}</b><small>${esc(d.current_ip || "手工节点")} · ${typeLabel(typeOf(d))} · ${statusText(d.status)}</small></div></div><div class="manager-device-actions"><button data-action="edit" data-id="${d.id}">详情</button>${d.is_new ? `<button data-action="seen" data-id="${d.id}">取消新标记</button>` : ""}<button data-action="visibility" data-id="${d.id}">${d.is_hidden ? "恢复显示" : "隐藏"}</button></div></div>`,
        )
        .join("")
    : '<p class="muted">没有符合条件的设备。</p>';
  list
    .querySelectorAll("button[data-action]")
    .forEach((button) => (button.onclick = () => managerAction(button)));
  list.querySelectorAll("input[data-select]").forEach(
    (input) =>
      (input.onchange = () => {
        const id = +input.dataset.select;
        input.checked ? selectedDevices.add(id) : selectedDevices.delete(id);
        renderManager();
      }),
  );
  appExtensions.managerRendered();
}
async function managerAction(button) {
  if (await appExtensions.managerAction(button)) return;
  const id = +button.dataset.id;
  const d = devices.find((x) => x.id === id);
  if (!d) return;
  if (button.dataset.action === "edit") {
    openDetail(id);
    return;
  }
  try {
    if (button.dataset.action === "seen")
      await api(`/api/devices/${id}`, {
        method: "PATCH",
        body: JSON.stringify({ is_new: false }),
      });
    if (button.dataset.action === "visibility")
      await api(`/api/devices/${id}/${d.is_hidden ? "unhide" : "hide"}`, {
        method: "POST",
      });
    await refresh();
    renderManager();
  } catch (e) {
    toast(e.message);
  }
}
async function runBatch(action) {
  const ids = [...selectedDevices];
  if (!ids.length) {
    toast("请先选择设备");
    return;
  }
  const body = { ids, action };
  if (action === "set_parent") {
    body.parent_id = +$("#batchParent").value;
    body.connection_type = $("#batchConnection").value;
    if (!body.parent_id) {
      toast("请选择父设备");
      return;
    }
  }
  try {
    const result = await api("/api/devices/batch", {
      method: "POST",
      body: JSON.stringify(body),
    });
    selectedDevices.clear();
    await refresh();
    renderManager();
    toast(`已更新 ${result.updated} 台设备`);
  } catch (e) {
    toast(e.message);
  }
}
async function openActivity() {
  await loadActivity();
}
async function loadActivity() {
  const scans = $("#scanHistoryList"),
    events = $("#statusEventList");
  scans.innerHTML = '<p class="activity-empty">正在读取…</p>';
  events.innerHTML = '<p class="activity-empty">正在读取…</p>';
  try {
    const [scanRows, eventRows] = await Promise.all([
      api("/api/scan/history"),
      api("/api/status/events?limit=100"),
    ]);
    scans.innerHTML = scanRows.length
      ? scanRows
          .map(
            (v) =>
              `<article class="activity-item"><b>${{ completed: "扫描完成", failed: "扫描失败", running: "正在扫描" }[v.status] || esc(v.status)}</b><small>${dateText(v.started_at)} · ${esc(v.cidrs)} · 已检查 ${v.scanned}/${v.total} 个地址 · 发现 ${v.found} 台设备</small>${v.error ? `<small>${esc(v.error)}</small>` : ""}</article>`,
          )
          .join("")
      : '<p class="activity-empty">还没有扫描记录。</p>';
    events.innerHTML = eventRows.length
      ? eventRows
          .map(
            (v) =>
              `<article class="activity-item"><b>${esc(v.device_name)}</b><small>${statusText(v.old_status)} → ${statusText(v.new_status)} · ${dateText(v.created_at)}</small></article>`,
          )
          .join("")
      : '<p class="activity-empty">还没有设备上线或离线记录。</p>';
  } catch (e) {
    scans.innerHTML = `<p class="activity-empty">${esc(e.message)}</p>`;
    events.innerHTML = '<p class="activity-empty">读取失败，请稍后重试。</p>';
  }
  appExtensions.activityLoaded();
}
