/* Focused UI extensions registered through the core's stable extension API. */
appExtensions.deviceLabel = (device) => (device.is_important ? "★ " : "");
appExtensions.deviceDetail = function (id) {
  const device = devices.find((item) => item.id === id);
  const deviceConnections = connections.filter(
    (item) => (item.TargetID || item.target_device_id) === id,
  );
  const editTitle = [...document.querySelectorAll("#detail h3")].find(
    (item) => item.textContent === "编辑",
  );
  const detailTerms = [...document.querySelectorAll("#detail .detail-grid dt")];
  const sourceTerm = detailTerms.find(
    (item) => item.textContent === "识别依据",
  );
  const sourceLabels = {
    hostname: "主机名",
    ports: "开放端口",
    mdns: "mDNS 服务",
    ssdp: "SSDP/UPnP",
    dhcp: "DHCP",
    mac: "MAC 厂商",
  };
  if (sourceTerm && device?.identification_source) {
    const sources = device.identification_source
      .split("+")
      .map((source) => sourceLabels[source] || source);
    sourceTerm.nextElementSibling.textContent = `${sources.join(" + ")} · ${Math.round((device.identification_confidence || 0) * 100)}%`;
  }
  if (editTitle && device?.identification_evidence?.length) {
    editTitle.insertAdjacentHTML(
      "beforebegin",
      `
      <section class="identification-card">
        <h3>为什么这样识别</h3>
        <ul>${device.identification_evidence.map((item) => `<li>${esc(item)}</li>`).join("")}</ul>
        <small>自动识别仅供参考，你手工设置的类型始终优先。</small>
      </section>`,
    );
  }
  if (
    editTitle &&
    device?.user_device_type &&
    device.user_device_type !== device.auto_device_type
  ) {
    editTitle.insertAdjacentHTML(
      "beforebegin",
      `<p class="device-correction">你已将自动识别的“${esc(typeLabel(device.auto_device_type))}”修正为“${esc(typeLabel(device.user_device_type))}”。喵拓会保留你的选择。</p>`,
    );
  }
  if (editTitle) {
    editTitle.insertAdjacentHTML(
      "beforebegin",
      `
      <section class="history-card">
        <div class="history-head"><h3>最近状态</h3><select id="historyRange"><option value="24">24 小时</option><option value="168">7 天</option><option value="720">30 天</option></select></div>
        <div id="historySummary" class="history-summary">正在读取记录…</div>
        <div id="historyChart" class="history-chart"></div>
      </section>`,
    );
    document.querySelector("#historyRange").onchange = (event) =>
      loadDeviceHistory(id, +event.target.value);
    loadDeviceHistory(id, 24);
  }
  const actions = document.querySelector("#detail .drawer-actions");
  if (actions) {
    actions.insertAdjacentHTML(
      "beforebegin",
      `<section class="attention-setting"><label>设备在线方式<select id="editPresence"><option value="normal" ${device?.presence_mode !== "occasional" ? "selected" : ""}>普通设备</option><option value="occasional" ${device?.presence_mode === "occasional" ? "selected" : ""}>偶尔在线（手机、平板、游戏机等）</option></select><small>偶尔在线设备休眠后会等待更久再判定离线，减少状态反复。</small></label><label class="check"><input id="editImportant" type="checkbox" ${device?.is_important ? "checked" : ""}>设为长期在线设备</label><small>适合路由器、NAS、服务器等通常不会关机的设备；只有这类设备离线时才会提醒你。</small>${device?.is_flapping ? '<p class="device-warning">这台设备最近一小时状态变化较频繁，喵拓已暂缓重复提醒。</p>' : ""}</section>`,
    );
  }
  const parentSelect = document.querySelector("#editParent");
  if (parentSelect) {
    parentSelect.value = "";
    parentSelect.closest("label").childNodes[0].textContent = "添加上级设备";
    const connectionTypeSelect = document.querySelector("#editConn");
    const connectionPortInput = document.querySelector("#editPort");
    if (connectionTypeSelect) connectionTypeSelect.value = "unknown";
    if (connectionPortInput) {
      connectionPortInput.value = "";
      connectionPortInput.closest("label").childNodes[0].textContent =
        "端口或说明";
    }
    const connectionList = deviceConnections
      .map((connection) => {
        const parentID = connection.SourceID || connection.source_device_id;
        const parent = devices.find((item) => item.id === parentID);
        const type = connection.Type || connection.connection_type || "unknown";
        const port = connection.Port || connection.port_label || "";
        const confirmed = connection.Confirmed ?? connection.user_confirmed;
        const sourceType =
          connection.SourceType ||
          connection.source_type ||
          (confirmed ? "manual" : "inferred");
        const confidence = Number(
          connection.Confidence ?? connection.confidence ?? 0,
        );
        const evidence = confirmed
          ? "你确认的连接 · 高可信度"
          : `${connectionSourceLabel(sourceType)} · ${confidence ? `可信度 ${Math.round(confidence * 100)}%` : "可信度未知"}`;
        return `<div class="connection-row"><div><b>${esc(parent ? nameOf(parent) : `设备 ${parentID}`)}</b><span>${esc(connectionTypeLabel(type))}${port ? ` · ${esc(port)}` : ""}</span><small>${esc(evidence)}</small></div><button type="button" data-remove-connection="${connection.ID || connection.id}">移除</button></div>`;
      })
      .join("");
    parentSelect
      .closest("label")
      .insertAdjacentHTML(
        "beforebegin",
        `<section class="connection-list"><h3>当前连接 <span>${deviceConnections.length}</span></h3>${connectionList || '<p class="muted">还没有上级连接。</p>'}</section>`,
      );
    document.querySelectorAll("[data-remove-connection]").forEach(
      (button) =>
        (button.onclick = async () => {
          if (!confirm("确定移除这条连接吗？其他连接不会受影响。")) return;
          try {
            await api(
              `/api/devices/${id}/connections/${button.dataset.removeConnection}`,
              { method: "DELETE" },
            );
            await refresh();
            openDetail(id);
            toast("连接已移除");
          } catch (error) {
            toast(error.message);
          }
        }),
    );
  }
  const connectionSelect = document.querySelector("#editConn");
  if (connectionSelect) {
    connectionSelect.insertAdjacentHTML(
      "afterend",
      '<small id="connectionHelp" class="connection-help"></small>',
    );
    const explain = () => {
      const descriptions = {
        unknown: "未知连接：还没有足够信息判断连接方式。",
        ethernet:
          "网线：表示设备通过有线网络连接；除非来源是设备协议，否则不代表已确认具体交换机端口。",
        wifi: "Wi-Fi：表示设备通过无线网络连接；不一定能确认具体接入点。",
        logical:
          "逻辑连接：只用于整理设备归属，不代表真实网线、无线接入点或交换机端口。",
        virtual:
          "虚拟连接：用于 Internet、虚拟机、容器或其他没有对应实体网线的关系。",
      };
      document.querySelector("#connectionHelp").textContent =
        `${descriptions[connectionSelect.value]} 选择上级设备并保存后，会新增一条连接，不会覆盖其他连接。`;
    };
    connectionSelect.onchange = explain;
    explain();
  }
};

appExtensions.saveDeviceDetail = async function (device) {
  const important = document.querySelector("#editImportant")?.checked || false;
  const presenceMode =
    document.querySelector("#editPresence")?.value || "normal";
  try {
    await api(`/api/devices/${device.id}`, {
      method: "PATCH",
      body: JSON.stringify({
        user_name: document.querySelector("#editName").value,
        user_device_type: document.querySelector("#editType").value,
        notes: document.querySelector("#editNotes").value,
        is_new: false,
        is_ignored: document.querySelector("#editIgnored").checked,
        always_show: document.querySelector("#editAlways").checked,
        is_important: important,
        presence_mode: presenceMode,
      }),
    });
    const parentID = +(document.querySelector("#editParent")?.value || 0);
    if (parentID) {
      await api(`/api/devices/${device.id}/connections`, {
        method: "POST",
        body: JSON.stringify({
          parent_id: parentID,
          connection_type: document.querySelector("#editConn").value,
          port_label: document.querySelector("#editPort").value,
        }),
      });
    }
    await refresh();
    openDetail(device.id);
    toast(parentID ? "设备信息和连接已保存" : "设备信息已保存");
  } catch (error) {
    toast(error.message);
  }
};

function connectionTypeLabel(type) {
  return (
    {
      unknown: "未知连接",
      ethernet: "网线",
      wifi: "Wi-Fi",
      logical: "逻辑连接",
      virtual: "虚拟连接",
    }[type] || "未知连接"
  );
}
function connectionSourceLabel(source) {
  return (
    {
      manual: "用户设置",
      inferred: "系统低可信度推测",
      lldp: "LLDP 发现",
      snmp: "SNMP 发现",
      controller: "网络控制器提供",
    }[source] || "来源未知"
  );
}

async function loadDeviceHistory(id, hours) {
  const summary = document.querySelector("#historySummary");
  const chart = document.querySelector("#historyChart");
  if (!summary || !chart) return;
  try {
    const data = await api(`/api/devices/${id}/history?hours=${hours}`);
    summary.innerHTML = `<b>${data.uptime_percent.toFixed(1)}%</b><span>在线率</span><b>${data.average_latency_ms ? data.average_latency_ms.toFixed(1) + " ms" : "—"}</b><span>平均延迟</span><b>${data.samples}</b><span>检查次数</span>`;
    if (!data.points.length) {
      chart.innerHTML =
        '<p class="muted">还没有历史数据，完成几次扫描后这里会出现曲线。</p>';
      return;
    }
    renderHistoryChart(chart, data.points);
  } catch (error) {
    summary.textContent = error.message;
    chart.innerHTML = "";
  }
}

function renderHistoryChart(container, points) {
  const items = points
    .map((point) => {
      const t = new Date(point.checked_at).getTime();
      if (Number.isNaN(t)) return null;
      const online = point.status === "online" && point.latency_ms > 0;
      return {
        t,
        checkedAt: point.checked_at,
        status: point.status,
        latency: online ? point.latency_ms : null,
      };
    })
    .filter(Boolean)
    .sort((a, b) => a.t - b.t);
  if (!items.length) {
    container.innerHTML = '<p class="muted">还没有可用的历史记录。</p>';
    return;
  }

  const viewW = 340,
    viewH = 112;
  const margin = { left: 46, right: 10, top: 12, bottom: 24 };
  const plotW = viewW - margin.left - margin.right;
  const plotH = viewH - margin.top - margin.bottom;

  let tMin = items[0].t,
    tMax = items[items.length - 1].t;
  if (tMax === tMin) {
    tMin -= 30 * 60000;
    tMax += 30 * 60000;
  }
  const span = tMax - tMin;
  const x = (t) => margin.left + ((t - tMin) / span) * plotW;
  const maxLatency = Math.max(10, ...items.map((item) => item.latency || 0));
  const y = (v) => margin.top + plotH - (v / maxLatency) * plotH;

  const stepPx = items.length > 1 ? plotW / (items.length - 1) : plotW;
  const segments = [];
  let current = [];
  items.forEach((item) => {
    if (item.latency === null) {
      if (current.length) segments.push(current);
      current = [];
    } else {
      current.push(`${x(item.t).toFixed(1)},${y(item.latency).toFixed(1)}`);
    }
  });
  if (current.length) segments.push(current);

  const offline = items
    .filter((item) => item.latency === null)
    .map((item) => {
      const cx = x(item.t);
      const w = Math.max(3, stepPx * 0.7);
      return `<rect x="${(cx - w / 2).toFixed(1)}" y="${margin.top}" width="${w.toFixed(1)}" height="${plotH}" rx="2"/>`;
    })
    .join("");

  const tickLabel = (t) => {
    const d = new Date(t);
    const hh = String(d.getHours()).padStart(2, "0");
    const mm = String(d.getMinutes()).padStart(2, "0");
    return span > 26 * 3600 * 1000
      ? `${d.getMonth() + 1}/${d.getDate()} ${hh}:${mm}`
      : `${hh}:${mm}`;
  };

  const axis = `<g class="history-axis">
    <line x1="${margin.left}" y1="${margin.top}" x2="${viewW - margin.right}" y2="${margin.top}"/>
    <text x="${margin.left - 6}" y="${margin.top + 3}" text-anchor="end">${maxLatency.toFixed(0)} ms</text>
    <line x1="${margin.left}" y1="${margin.top + plotH}" x2="${viewW - margin.right}" y2="${margin.top + plotH}"/>
    <text x="${margin.left - 6}" y="${margin.top + plotH + 3}" text-anchor="end">0</text>
    <text x="${margin.left}" y="${viewH - 8}">${tickLabel(tMin)}</text>
    <text x="${(margin.left + viewW - margin.right) / 2}" y="${viewH - 8}" text-anchor="middle">${tickLabel(tMin + span / 2)}</text>
    <text x="${viewW - margin.right}" y="${viewH - 8}" text-anchor="end">${tickLabel(tMax)}</text>
  </g>`;

  container.style.position = "relative";
  container.innerHTML =
    `<svg viewBox="0 0 ${viewW} ${viewH}" role="img" aria-label="设备延迟历史">` +
    `<g class="history-offline">${offline}</g>` +
    `${axis}` +
    segments.map((pts) => `<polyline points="${pts.join(" ")}"/>`).join("") +
    `</svg>` +
    `<div class="history-tooltip" hidden></div>` +
    `<div class="history-legend"><span>绿色：延迟</span><span>红色：未在线</span></div>`;

  const svg = container.querySelector("svg");
  const tip = container.querySelector(".history-tooltip");
  const statusLabel = {
    online: "在线",
    suspected_offline: "疑似离线",
    offline: "离线",
    unknown: "未知",
  };
  svg.addEventListener("mousemove", (event) => {
    const rect = svg.getBoundingClientRect();
    const px = ((event.clientX - rect.left) / rect.width) * viewW;
    if (px < margin.left || px > viewW - margin.right) {
      tip.hidden = true;
      return;
    }
    const t = tMin + ((px - margin.left) / plotW) * span;
    let nearest = items[0];
    let best = Infinity;
    for (const item of items) {
      const distance = Math.abs(item.t - t);
      if (distance < best) {
        best = distance;
        nearest = item;
      }
    }
    const detail = nearest.latency !== null ? ` · ${nearest.latency.toFixed(1)} ms` : "";
    tip.textContent = `${new Date(nearest.checkedAt).toLocaleString()} · ${statusLabel[nearest.status] || nearest.status}${detail}`;
    const left = Math.min(Math.max(0, px - 70), rect.width - 150);
    tip.style.left = `${left}px`;
    tip.style.top = `${Math.max(0, event.clientY - rect.top - 34)}px`;
    tip.hidden = false;
  });
  svg.addEventListener("mouseleave", () => {
    tip.hidden = true;
  });
}
