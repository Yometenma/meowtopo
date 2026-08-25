/* Setup wizard, scanning, notifications and settings core. */
async function wizard() {
  const d = await api("/api/network/detection"),
    interfaces = d.interfaces || [],
    gateways = d.gateways || [],
    warnings = d.warnings || [];
  $("#interfaceSelect").innerHTML =
    '<option value="">自动选择</option>' +
    interfaces
      .map(
        (i) =>
          `<option value="${attr(i.name)}">${esc(i.name)} · ${esc(i.ip)} · ${esc(i.cidr)}${i.virtual ? "（虚拟）" : ""}</option>`,
      )
      .join("");
  const best =
    interfaces.find((i) => i.recommended) ||
    interfaces.find((i) => !i.virtual) ||
    interfaces[0];
  if (best) {
    $("#interfaceSelect").value = best.name;
    $("#cidrInput").value = best.cidr;
  }
  if (gateways[0]) $("#gatewayInput").value = gateways[0];
  $("#detectBox").innerHTML =
    `发现 ${interfaces.length} 个可用 IPv4 接口；${d.data_writable ? "数据目录可写" : "数据目录不可写"}。${warnings.map(esc).join("<br>")}`;
  $("#wizard").showModal();
}
async function saveWizard(e) {
  e.preventDefault();
  $("#wizardError").textContent = "";
  try {
    const v = {
      initialized: true,
      scan_interface: $("#interfaceSelect").value,
      scan_cidrs: $("#cidrInput").value,
      gateway_ip: $("#gatewayInput").value,
      scan_interval: $("#intervalInput").value,
      offline_threshold: +$("#thresholdInput").value,
      enable_port_scan: $("#portScanInput").checked,
    };
    await api("/api/settings", { method: "PATCH", body: JSON.stringify(v) });
    settings = { ...settings, ...v };
    $("#wizard").close();
    if ($("#firstScanInput").checked) startScan();
    else refresh();
  } catch (x) {
    $("#wizardError").textContent = x.message;
  }
}
async function startScan() {
  const button = $("#scanBtn");
  button.disabled = true;
  button.textContent = "正在启动…";
  try {
    await api("/api/scan", {
      method: "POST",
      body: JSON.stringify({
        cidr: settings.scan_cidrs || $("#setCIDR").value,
      }),
    });
    showProgress(await api("/api/scan/status"));
  } catch (e) {
    button.disabled = false;
    button.textContent = "立即扫描";
    toast(e.message);
  }
}
function showProgress(s) {
  const running = !!s.running,
    button = $("#scanBtn");
  $("#scanProgress").classList.toggle("hidden", !running);
  button.disabled = running;
  button.textContent = running ? "扫描中…" : "立即扫描";
  button.setAttribute("aria-busy", String(running));
  if (running) {
    $("#progressText").textContent =
      `${s.scanned}/${s.total} · 发现 ${s.found}`;
    $("#progressBar").max = s.total || 1;
    $("#progressBar").value = s.scanned;
  }
}
function connectEvents() {
  const es = new EventSource("/api/events"),
    foot = document.querySelector(".sidebar-foot");
  es.onopen = () => {
    if (foot) {
      foot.classList.remove("disconnected");
      foot.querySelector("span:last-child").textContent = "本地运行中";
    }
  };
  es.onerror = () => {
    if (foot) {
      foot.classList.add("disconnected");
      foot.querySelector("span:last-child").textContent = "正在重新连接";
    }
  };
  es.onmessage = (e) => {
    const v = JSON.parse(e.data);
    if (v.type === "scan_progress" || v.type === "scan_started")
      showProgress(v.data);
    if (
      [
        "device_seen",
        "device_updated",
        "devices_updated",
        "device_deleted",
        "topology_changed",
      ].includes(v.type)
    )
      refresh();
    if (v.type === "scan_completed") {
      showProgress(v.data);
      refresh();
      toast(v.data.error ? "扫描未完整完成" : "扫描完成");
    }
  };
}
function notificationValues() {
  return {
    notification_enabled: $("#notifyEnabled").checked,
    notification_telegram_enabled: $("#notifyTelegram").checked,
    notification_telegram_token: $("#notifyToken").value,
    notification_telegram_chat_id: $("#notifyChat").value,
    notification_webhook_enabled: $("#notifyWebhook").checked,
    notification_webhook_url: $("#notifyWebhookURL").value,
    notification_new_device: $("#notifyNew").checked,
    notification_offline: $("#notifyOffline").checked,
    notification_online: $("#notifyOnline").checked,
    notification_scan_error: $("#notifyError").checked,
    ...appExtensions.notificationValues(),
  };
}
function ensureNotificationUI() {
  if ($("#notificationPane")) return;
  $("#backupTab").insertAdjacentHTML(
    "beforebegin",
    '<button type="button" id="notificationTab">外部通知</button>',
  );
  $("#settingsMain").insertAdjacentHTML(
    "afterend",
    `<div id="notificationPane" class="hidden pane settings-groups">
    <section class="settings-group notification-master">
      <div class="settings-group-title"><b>通知总开关</b><small>关闭后不会向任何外部位置发送消息</small></div>
      <label class="check"><input id="notifyEnabled" type="checkbox">启用外部通知</label>
    </section>
    <section class="settings-group">
      <div class="settings-group-title"><b>Telegram</b><small>推送到个人、群组或频道</small></div>
      <label class="check"><input id="notifyTelegram" type="checkbox">使用 Telegram</label>
      <div class="formgrid"><label>Bot Token<input id="notifyToken" type="password" autocomplete="new-password" placeholder="从 BotFather 获取"></label><label>Chat ID<input id="notifyChat" placeholder="个人、群组或频道 ID"></label></div>
    </section>
    <section class="settings-group">
      <div class="settings-group-title"><b>通用 Webhook</b><small>发送到支持 Webhook 的自建服务或自动化工具</small></div>
      <label class="check"><input id="notifyWebhook" type="checkbox">使用 Webhook</label>
      <label>Webhook 地址<input id="notifyWebhookURL" type="url" placeholder="https://example.com/webhook"></label>
    </section>
    <section class="settings-group">
      <div class="settings-group-title"><b>通知内容</b><small>只选择真正需要知道的变化，减少打扰</small></div>
      <div class="notification-events"><label class="check"><input id="notifyNew" type="checkbox">发现新设备</label><label class="check"><input id="notifyOffline" type="checkbox">长期在线设备离线</label><label class="check"><input id="notifyOnline" type="checkbox">设备重新上线</label><label class="check"><input id="notifyError" type="checkbox">扫描异常</label></div>
    </section>
    <div class="notification-test-row"><button type="button" id="notificationTest" class="primary">保存并发送测试消息</button><p id="notificationResult" class="muted" role="status"></p></div>
  </div>`,
  );
  $("#notificationTab").onclick = () => showPane("notification");
  $("#notificationTest").onclick = testNotification;
  appExtensions.notificationUI();
}
async function openSettings() {
  settings = await api("/api/settings");
  const detection = await api("/api/network/detection");
  const selected = settings.scan_interface || "";
  $("#setInterface").innerHTML =
    '<option value="">自动选择</option>' +
    detection.interfaces
      .map(
        (i) =>
          `<option value="${attr(i.name)}">${esc(i.name)} · ${esc(i.ip)} · ${esc(i.cidr)}${i.virtual ? "（虚拟）" : ""}</option>`,
      )
      .join("");
  $("#setInterface").value = selected;
  $("#setCIDR").value = settings.scan_cidrs || "";
  $("#setGateway").value = settings.gateway_ip || "";
  $("#setInterval").value = settings.scan_interval || "5m";
  $("#setPingTimeout").value = settings.ping_timeout || "800ms";
  $("#setTCPTimeout").value = settings.tcp_timeout || "350ms";
  $("#setConcurrency").value = settings.scan_concurrency || 32;
  $("#setThreshold").value = settings.offline_threshold || 3;
  $("#setPortScan").checked = settings.enable_port_scan !== "false";
  $("#notifyEnabled").checked = settings.notification_enabled === "true";
  $("#notifyTelegram").checked =
    settings.notification_telegram_enabled === "true";
  $("#notifyToken").value = settings.notification_telegram_token || "";
  $("#notifyChat").value = settings.notification_telegram_chat_id || "";
  $("#notifyWebhook").checked =
    settings.notification_webhook_enabled === "true";
  $("#notifyWebhookURL").value = settings.notification_webhook_url || "";
  $("#notifyNew").checked = settings.notification_new_device !== "false";
  $("#notifyOffline").checked = settings.notification_offline !== "false";
  $("#notifyOnline").checked = settings.notification_online !== "false";
  $("#notifyError").checked = settings.notification_scan_error !== "false";
  $("#notificationResult").textContent = "";
  $("#settingsError").textContent = "";
  showPane("main");
  $("#settingsDialog").show();
  await appExtensions.settingsOpened();
}
async function saveSettings(e) {
  e.preventDefault();
  try {
    const v = {
      scan_interface: $("#setInterface").value,
      scan_cidrs: $("#setCIDR").value,
      gateway_ip: $("#setGateway").value,
      scan_interval: $("#setInterval").value,
      ping_timeout: $("#setPingTimeout").value,
      tcp_timeout: $("#setTCPTimeout").value,
      scan_concurrency: +$("#setConcurrency").value,
      offline_threshold: +$("#setThreshold").value,
      enable_port_scan: $("#setPortScan").checked,
      ...notificationValues(),
    };
    await api("/api/settings", { method: "PATCH", body: JSON.stringify(v) });
    settings = { ...settings, ...v };
    $("#settingsDialog").close();
    toast("设置已保存并生效");
  } catch (x) {
    $("#settingsError").textContent = x.message;
  }
}
async function testNotification() {
  const result = $("#notificationResult");
  result.textContent = "正在发送测试消息…";
  try {
    await api("/api/settings", {
      method: "PATCH",
      body: JSON.stringify(notificationValues()),
    });
    await api("/api/notifications/test", { method: "POST" });
    result.textContent = "测试消息发送成功，请检查接收端。";
  } catch (e) {
    result.textContent = e.message;
  }
}
