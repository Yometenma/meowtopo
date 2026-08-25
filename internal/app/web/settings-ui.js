/* Settings, notification, maintenance and diagnostics UI. */
appExtensions.notificationValues = function () {
  return {
    notification_cooldown:
      document.querySelector("#notifyCooldown")?.value || "15m",
    notification_important_only: true,
    automatic_backup_enabled:
      document.querySelector("#autoBackupEnabled")?.checked || false,
    automatic_backup_interval:
      document.querySelector("#autoBackupInterval")?.value || "24h",
    automatic_backup_keep: +(
      document.querySelector("#autoBackupKeep")?.value || 7
    ),
    history_retention_days: +(
      document.querySelector("#historyRetentionDays")?.value || 30
    ),
  };
};

appExtensions.notificationUI = function () {
  if (!document.querySelector("#notifyCooldown")) {
    document
      .querySelector("#notificationResult")
      .insertAdjacentHTML(
        "beforebegin",
        `<div class="formgrid notification-controls"><label>同类消息冷却时间<select id="notifyCooldown"><option value="0s">不限制</option><option value="5m">5 分钟</option><option value="15m">15 分钟</option><option value="1h">1 小时</option><option value="6h">6 小时</option></select></label><p class="muted notification-rule">离线与恢复通知只针对标记为「长期在线」的设备，其他设备的变化不会触发提醒。</p></div>`,
      );
  }
};

appExtensions.settingsOpened = async function () {
  document.querySelector("#notifyCooldown").value =
    settings.notification_cooldown || "15m";
  document.querySelector("#autoBackupEnabled").checked =
    settings.automatic_backup_enabled === "true";
  document.querySelector("#autoBackupInterval").value =
    settings.automatic_backup_interval || "24h";
  document.querySelector("#autoBackupKeep").value =
    settings.automatic_backup_keep || 7;
  document.querySelector("#historyRetentionDays").value =
    settings.history_retention_days || 30;
  loadMaintenanceStatus();
  document.querySelector("#setCIDR").placeholder =
    "多个网段用逗号分隔，例如 192.168.1.0/24,10.0.0.0/24";
  loadScanDiagnostics();
  await appExtensions.snmpOpened();
};

function ensureMaintenanceUI() {
  const pane = document.querySelector("#backupPane");
  if (!pane || document.querySelector("#autoBackupEnabled")) return;
  pane.insertAdjacentHTML(
    "beforeend",
    `
    <hr><h3>服务器自动备份</h3>
    <p class="muted">备份保存在 MeowTopo 数据目录的 backups 文件夹，不需要额外程序。</p>
    <div class="formgrid">
      <label class="check"><input id="autoBackupEnabled" type="checkbox">启用自动备份</label>
      <label>备份间隔<select id="autoBackupInterval"><option value="12h">每 12 小时</option><option value="24h">每天</option><option value="72h">每 3 天</option><option value="168h">每周</option></select></label>
      <label>保留备份份数<input id="autoBackupKeep" type="number" min="1" max="30" value="7"></label>
      <label>历史记录保留天数<input id="historyRetentionDays" type="number" min="7" max="365" value="30"></label>
    </div>
    <button type="button" id="backupNow">立即在服务器创建一份</button>
    <p id="maintenanceStatus" class="muted">正在读取备份状态…</p>`,
  );
  document.querySelector("#backupNow").onclick = async () => {
    const result = document.querySelector("#maintenanceStatus");
    result.textContent = "正在创建备份…";
    try {
      await api("/api/maintenance/backup", { method: "POST" });
      toast("服务器备份已创建");
      loadMaintenanceStatus();
    } catch (error) {
      result.textContent = error.message;
    }
  };
}
function ensureQualityTools() {
  const settingsMain = document.querySelector("#settingsMain");
  if (settingsMain && !document.querySelector("#scanDiagnostics")) {
    settingsMain.insertAdjacentHTML(
      "beforeend",
      `<section class="settings-group quality-tools"><div class="settings-group-title"><b>扫描情况</b><small>了解为什么有些设备可能没有被发现</small></div><div id="scanDiagnostics" class="scan-diagnostics">打开设置后会读取最近一次扫描情况。</div><button type="button" id="refreshDiagnostics">重新检查</button></section>`,
    );
    document.querySelector("#refreshDiagnostics").onclick = loadScanDiagnostics;
  }
  const toolbar = document.querySelector("#managerDialog .manager-toolbar");
  if (toolbar && !document.querySelector("#deviceExport")) {
    toolbar.insertAdjacentHTML(
      "beforeend",
      `<div id="deviceExport" class="export-actions"><span>导出设备清单</span><a class="button" href="/api/devices/export?format=csv">CSV</a><a class="button" href="/api/devices/export?format=json">JSON</a></div>`,
    );
  }
}

async function loadScanDiagnostics() {
  const target = document.querySelector("#scanDiagnostics");
  if (!target) return;
  target.textContent = "正在检查…";
  try {
    const data = await api("/api/scan/diagnostics");
    const latest = data.latest || {};
    const summary = latest.total_addresses
      ? `最近扫描 ${latest.scanned_addresses}/${latest.total_addresses} 个地址，发现 ${latest.found_devices} 台设备。`
      : "还没有扫描记录。";
    const warnings = data.warnings?.length
      ? `<ul>${data.warnings.map((item) => `<li>${esc(item)}</li>`).join("")}</ul>`
      : '<p class="diagnostic-ok">没有发现明显的扫描环境问题。</p>';
    target.innerHTML = `<p><b>${esc(summary)}</b></p><p>网段：${esc(data.configured_cidrs || "未设置")} · 网卡：${esc(data.interface || "自动选择")}</p>${warnings}`;
  } catch (error) {
    target.textContent = error.message;
  }
}
