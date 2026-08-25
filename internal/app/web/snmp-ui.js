/* Managed SNMP devices and LLDP discovery settings. */
let snmpTargets = [];

appExtensions.snmpOpened = async function () {
  await loadSNMPTargets();
};

appExtensions.snmpBind = function () {
  $("#snmpAdd").onclick = () => openSNMPTarget();
  $("#snmpPoll").onclick = pollSNMPTargets;
  $("#snmpVersion").onchange = updateSNMPFields;
  $("#snmpSecurity").onchange = updateSNMPFields;
  $("#snmpClose").onclick = $("#snmpCancel").onclick = () =>
    $("#snmpDialog").close();
  $("#snmpForm").addEventListener("submit", saveSNMPTarget);
  $("#snmpTargets").addEventListener("click", handleSNMPAction);
};

async function loadSNMPTargets() {
  const list = $("#snmpTargets");
  if (!list) return;
  list.innerHTML = '<p class="muted">正在读取受管设备…</p>';
  try {
    const data = await api("/api/snmp/targets");
    snmpTargets = data.targets || [];
    renderSNMPTargets(data.status || {});
  } catch (error) {
    list.innerHTML = `<p class="error">${esc(error.message)}</p>`;
  }
}

function renderSNMPTargets(status) {
  $("#snmpSummary").textContent = status.running
    ? "正在读取…"
    : status.finished_at
      ? `上次读取：${dateText(status.finished_at)} · ${status.successful || 0}/${status.targets || 0} 台成功 · ${status.neighbors || 0} 个 LLDP 邻居`
      : `${snmpTargets.length} 台受管设备`;
  $("#snmpTargets").innerHTML = snmpTargets.length
    ? snmpTargets
        .map(
          (target) => `<article class="snmp-target ${target.enabled ? "" : "disabled"}">
            <div><b>${esc(target.address)}:${target.port}</b><span>SNMP v${esc(target.version)}${target.version === "3" ? ` · ${esc(target.security_level)}` : ""}</span><small class="${target.last_status === "failed" ? "error" : "muted"}">${target.last_status === "ok" ? `连接正常 · ${dateText(target.last_polled_at)}` : target.last_status === "partial" ? `SNMP 正常，LLDP 不可用 · ${esc(target.last_error)}` : target.last_error ? esc(target.last_error) : "尚未测试"}</small></div>
            <div><button type="button" data-snmp="test" data-id="${target.id}">测试</button><button type="button" data-snmp="edit" data-id="${target.id}">编辑</button><button type="button" data-snmp="delete" data-id="${target.id}">删除</button></div>
          </article>`,
        )
        .join("")
    : '<div class="empty-state"><b>还没有受管设备</b><p>如果交换机或路由器支持 SNMP，可添加它来读取 LLDP 邻居与端口。</p></div>';
}

function openSNMPTarget(target = null) {
  $("#snmpForm").reset();
  $("#snmpID").value = target?.id || "";
  $("#snmpDialogTitle").textContent = target ? "编辑受管网络设备" : "添加受管网络设备";
  $("#snmpAddress").value = target?.address || "";
  $("#snmpPort").value = target?.port || 161;
  $("#snmpVersion").value = target?.version || "2c";
  $("#snmpCommunity").value = "";
  $("#snmpCommunity").placeholder = target?.community_configured ? "已保存；留空表示不修改" : "例如设备设置的只读 community";
  $("#snmpUsername").value = target?.username || "";
  $("#snmpSecurity").value = target?.security_level || "authPriv";
  $("#snmpAuthProtocol").value = target?.auth_protocol || "SHA256";
  $("#snmpAuthPassword").value = "";
  $("#snmpAuthPassword").placeholder = target?.auth_password_configured ? "已保存；留空表示不修改" : "至少 8 个字符";
  $("#snmpPrivacyProtocol").value = target?.privacy_protocol || "AES";
  $("#snmpPrivacyPassword").value = "";
  $("#snmpPrivacyPassword").placeholder = target?.privacy_password_configured ? "已保存；留空表示不修改" : "至少 8 个字符";
  $("#snmpEnabled").checked = target ? target.enabled : true;
  $("#snmpFormError").textContent = "";
  updateSNMPFields();
  $("#snmpDialog").showModal();
}

function updateSNMPFields() {
  const v3 = $("#snmpVersion").value === "3";
  document.querySelectorAll(".snmp-v3").forEach((field) => field.classList.toggle("hidden", !v3));
  document.querySelectorAll(".snmp-community").forEach((field) => field.classList.toggle("hidden", v3));
  const level = $("#snmpSecurity").value;
  document.querySelectorAll(".snmp-auth").forEach((field) => field.classList.toggle("hidden", !v3 || level === "noAuthNoPriv"));
  document.querySelectorAll(".snmp-privacy").forEach((field) => field.classList.toggle("hidden", !v3 || level !== "authPriv"));
}

async function saveSNMPTarget(event) {
  event.preventDefault();
  const id = $("#snmpID").value;
  const payload = {
    address: $("#snmpAddress").value.trim(),
    port: +$("#snmpPort").value,
    version: $("#snmpVersion").value,
    community: $("#snmpCommunity").value,
    security_level: $("#snmpSecurity").value,
    username: $("#snmpUsername").value.trim(),
    auth_protocol: $("#snmpAuthProtocol").value,
    auth_password: $("#snmpAuthPassword").value,
    privacy_protocol: $("#snmpPrivacyProtocol").value,
    privacy_password: $("#snmpPrivacyPassword").value,
    enabled: $("#snmpEnabled").checked,
  };
  try {
    await api(id ? `/api/snmp/targets/${id}` : "/api/snmp/targets", { method: id ? "PATCH" : "POST", body: JSON.stringify(payload) });
    $("#snmpDialog").close();
    toast("受管设备已保存");
    await loadSNMPTargets();
  } catch (error) {
    $("#snmpFormError").textContent = error.message;
  }
}

async function handleSNMPAction(event) {
  const button = event.target.closest("button[data-snmp]");
  if (!button) return;
  const id = +button.dataset.id;
  const target = snmpTargets.find((item) => item.id === id);
  if (button.dataset.snmp === "edit") return openSNMPTarget(target);
  if (button.dataset.snmp === "delete") {
    if (!confirm(`确定删除 ${target.address} 的 SNMP 设置吗？已经发现的设备和连接不会被删除。`)) return;
    await api(`/api/snmp/targets/${id}`, { method: "DELETE" });
    toast("受管设备设置已删除");
    return loadSNMPTargets();
  }
  button.disabled = true;
  button.textContent = "测试中…";
  try {
    const result = await api(`/api/snmp/targets/${id}/test`, { method: "POST" });
    toast(`连接成功：${result.name || target.address}`);
  } catch (error) {
    toast(error.message);
  } finally {
    await loadSNMPTargets();
  }
}

async function pollSNMPTargets() {
  const button = $("#snmpPoll");
  button.disabled = true;
  $("#snmpSummary").textContent = "正在读取 SNMP 与 LLDP…";
  try {
    const status = await api("/api/snmp/poll", { method: "POST" });
    toast(`读取完成：${status.successful}/${status.targets} 台成功，发现 ${status.neighbors} 个邻居`);
    await refresh();
  } catch (error) {
    toast(error.message);
  } finally {
    button.disabled = false;
    await loadSNMPTargets();
  }
}
