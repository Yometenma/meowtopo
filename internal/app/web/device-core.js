/* Device detail rendering and shared presentation helpers. */
function openDetail(id) {
  const d = devices.find((x) => x.id === id);
  if (!d) return;
  const conn = connections.find((c) => c.target_device_id === id);
  const parentId = conn ? conn.source_device_id : 0;
  const connType = conn ? conn.connection_type : "unknown";
  const connPort = conn ? conn.port_label : "";
  const parents = devices
    .filter((x) => x.id !== id)
    .map(
      (x) =>
        `<option value="${x.id}" ${x.id === parentId ? "selected" : ""}>${esc(nameOf(x))}</option>`,
    )
    .join("");
  const type = typeOf(d);
  const confidence = d.identification_confidence
    ? `${Math.round(d.identification_confidence * 100)}%`
    : "—";
  const source =
    { hostname: "主机名", ports: "开放端口" }[d.identification_source] || "—";
  const ports = (d.open_ports || []).join(", ") || "—";
  const connOptions = ["unknown", "ethernet", "wifi", "logical", "virtual"]
    .map(
      (v) =>
        `<option value="${v}" ${v === connType ? "selected" : ""}>${connectionTypeLabel(v)}</option>`,
    )
    .join("");
  $("#detail").innerHTML =
    `<div class="device-title"><div class="device-icon"><img src="${deviceArtwork(type)}" alt=""></div><div><h2>${esc(nameOf(d))}</h2><span class="muted">${esc(d.current_ip || "手工节点")} · ${statusText(d.status)}</span></div></div><dl class="detail-grid"><dt>自动名称</dt><dd>${esc(d.auto_hostname || "—")}</dd><dt>MAC</dt><dd>${esc(d.mac_address || "—")}</dd><dt>厂商</dt><dd>${esc(d.vendor || "未知")}</dd><dt>自动类型</dt><dd>${esc(typeLabel(d.auto_device_type))}</dd><dt>识别依据</dt><dd id="identificationSource">${source} · ${confidence}</dd><dt>开放端口</dt><dd>${esc(ports)}</dd><dt>探测方式</dt><dd>${esc(probeLabel(d.probe_method))}</dd><dt>延迟</dt><dd>${d.ping_latency_ms ? d.ping_latency_ms.toFixed(1) + " ms" : "—"}</dd><dt>首次发现</dt><dd>${dateText(d.first_seen_at)}</dd><dt>最后在线</dt><dd>${dateText(d.last_seen_at)}</dd><dt>最近检查</dt><dd>${dateText(d.last_checked_at)}</dd><dt>数据来源</dt><dd>${d.created_manually ? "手工创建" : "自动发现"}</dd></dl><h3 id="editHeading">编辑</h3><label>显示名称<input id="editName" value="${attr(d.user_name)}"></label><label>设备类型<select id="editType">${typeOptions(type)}</select></label><label>备注<textarea id="editNotes">${esc(d.notes || "")}</textarea></label><label>父设备<select id="editParent"><option value="">未分配</option>${parents}</select></label><label>连接方式<select id="editConn">${connOptions}</select></label><label>端口名称<input id="editPort" value="${attr(connPort)}"></label><label class="check"><input id="editIgnored" type="checkbox" ${d.is_ignored ? "checked" : ""}>忽略离线判定</label><label class="check"><input id="editAlways" type="checkbox" ${d.always_show ? "checked" : ""}>始终显示</label><div class="drawer-actions"><button class="primary" id="editSave">保存</button><button id="pingOne" ${d.current_ip ? "" : "disabled"}>探测</button><button id="openOne" ${d.current_ip ? "" : "disabled"}>打开页面</button><button id="lockOne">${d.locked ? "取消固定" : "固定位置"}</button><button id="hideOne">${d.is_hidden ? "恢复显示" : "隐藏"}</button>${d.created_manually ? '<button id="deleteOne">删除节点</button>' : ""}</div>`;
  $("#drawer").classList.add("open");
  $("#editSave").onclick = () => appExtensions.saveDeviceDetail(d);
  $("#pingOne").onclick = async () => {
    try {
      const x = await api(`/api/devices/${id}/ping`, { method: "POST" });
      toast(
        x.reachable
          ? `设备可达 · ${probeLabel(x.method)} · ${x.latency_ms.toFixed(1)} ms`
          : "暂未探测到设备",
      );
    } catch (e) {
      toast(e.message);
    }
  };
  $("#openOne").onclick = () =>
    window.open(`http://${d.current_ip}`, "_blank", "noopener");
  $("#lockOne").onclick = async () => {
    const n = cy.$id(String(id));
    d.locked ? n.unlock() : n.lock();
    await api(`/api/devices/${id}/position`, {
      method: "PATCH",
      body: JSON.stringify({
        x: n.position("x"),
        y: n.position("y"),
        locked: !d.locked,
      }),
    });
    d.locked = !d.locked;
    openDetail(id);
  };
  $("#hideOne").onclick = async () => {
    await api(`/api/devices/${id}/${d.is_hidden ? "unhide" : "hide"}`, {
      method: "POST",
    });
    $("#drawer").classList.remove("open");
    await refresh();
    renderManager();
  };
  if (d.created_manually)
    $("#deleteOne").onclick = async () => {
      if (!confirm(`确定删除手工节点“${nameOf(d)}”吗？`)) return;
      await api(`/api/devices/${id}`, { method: "DELETE" });
      $("#drawer").classList.remove("open");
      await refresh();
      renderManager();
      toast("手工节点已删除");
    };
  appExtensions.deviceDetail(id);
}

function statusText(v) {
  return (
    {
      online: "在线",
      suspected_offline: "疑似离线",
      offline: "离线",
      unknown: "未知",
    }[v] || v
  );
}
function dateText(v) {
  return v ? new Date(v).toLocaleString() : "—";
}
function probeLabel(v) {
  return (
    { icmp: "ICMP", tcp_connect: "TCP", "icmp+tcp_connect": "ICMP + TCP", snmp: "SNMP" }[v] ||
    "—"
  );
}
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
function typeLabel(v) {
  return (
    {
      gateway: "网关",
      router: "路由器",
      ap: "无线 AP",
      switch: "交换机",
      nas: "NAS",
      linux: "Linux 服务器",
      windows: "Windows 电脑",
      macos: "macOS 电脑",
      phone: "手机",
      tablet: "平板",
      tv: "电视",
      camera: "智能家居",
      iot: "智能家居",
      game: "游戏设备",
      printer: "打印机",
      room: "房间",
      unknown: "未知设备",
      internet: "Internet",
      modem: "光猫",
    }[v] || "未知设备"
  );
}
function esc(v) {
  const e = document.createElement("span");
  e.textContent = v ?? "";
  return e.innerHTML;
}
function attr(v) {
  return esc(v).replaceAll('"', "&quot;");
}
function typeOptions(sel) {
  return Object.keys(icons)
    .filter((v) => v !== "camera")
    .map(
      (v) =>
        `<option value="${v}" ${v === sel ? "selected" : ""}>${typeLabel(v)}</option>`,
    )
    .join("");
}
