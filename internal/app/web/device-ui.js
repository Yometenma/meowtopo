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
    const values = data.points.map((point) =>
      point.status === "online" && point.latency_ms > 0
        ? point.latency_ms
        : null,
    );
    const maximum = Math.max(10, ...values.filter((value) => value !== null));
    const width = 320,
      height = 90,
      step = values.length > 1 ? width / (values.length - 1) : width;
    const segments = [];
    let current = [];
    values.forEach((value, index) => {
      if (value === null) {
        if (current.length) segments.push(current);
        current = [];
      } else {
        current.push(
          `${(index * step).toFixed(1)},${(height - (value / maximum) * (height - 12)).toFixed(1)}`,
        );
      }
    });
    if (current.length) segments.push(current);
    const offline = values
      .map((value, index) =>
        value === null
          ? `<rect x="${Math.max(0, index * step - 2)}" y="0" width="4" height="${height}" rx="2"/>`
          : "",
      )
      .join("");
    chart.innerHTML = `<svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" role="img" aria-label="设备延迟历史"><g class="history-offline">${offline}</g>${segments.map((points) => `<polyline points="${points.join(" ")}"/>`).join("")}</svg><div class="history-legend"><span>绿色：延迟</span><span>红色：未在线</span></div>`;
  } catch (error) {
    summary.textContent = error.message;
    chart.innerHTML = "";
  }
}
