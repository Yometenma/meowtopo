/* Feature extensions kept separate from the compact core UI bundle. */
const originalOpenDetail = openDetail;
const originalNodeDisplay = nodeDisplay;
nodeDisplay = device => `${device.is_important ? '★ ' : ''}${originalNodeDisplay(device)}`;
openDetail = function (id) {
  originalOpenDetail(id);
  const device = devices.find(item => item.id === id);
  const connection = connections.find(item => (item.TargetID || item.target_device_id) === id);
  const editTitle = [...document.querySelectorAll('#detail h3')].find(item => item.textContent === '编辑');
  if (editTitle) {
    editTitle.insertAdjacentHTML('beforebegin', `
      <section class="history-card">
        <div class="history-head"><h3>最近状态</h3><select id="historyRange"><option value="24">24 小时</option><option value="168">7 天</option><option value="720">30 天</option></select></div>
        <div id="historySummary" class="history-summary">正在读取记录…</div>
        <div id="historyChart" class="history-chart"></div>
      </section>`);
    document.querySelector('#historyRange').onchange = event => loadDeviceHistory(id, +event.target.value);
    loadDeviceHistory(id, 24);
  }
  const actions = document.querySelector('#detail .drawer-actions');
  if (actions) {
    actions.insertAdjacentHTML('beforebegin', `<section class="attention-setting"><label>设备在线方式<select id="editPresence"><option value="normal" ${device?.presence_mode !== 'occasional' ? 'selected' : ''}>普通设备</option><option value="occasional" ${device?.presence_mode === 'occasional' ? 'selected' : ''}>偶尔在线（手机、平板、游戏机等）</option></select><small>偶尔在线设备休眠后会等待更久再判定离线，减少状态反复。</small></label><label class="check"><input id="editImportant" type="checkbox" ${device?.is_important ? 'checked' : ''}>设为长期在线设备</label><small>适合路由器、NAS、服务器等通常不会关机的设备；只有这类设备离线时才会提醒你。</small>${device?.is_flapping ? '<p class="device-warning">这台设备最近一小时状态变化较频繁，喵拓已暂缓重复提醒。</p>' : ''}</section>`);
  }
  const connectionSelect = document.querySelector('#editConn');
  if (connectionSelect) {
    connectionSelect.insertAdjacentHTML('afterend', '<small id="connectionHelp" class="connection-help"></small>');
    const explain = () => {
      const descriptions = {unknown: '未知连接：还没有足够信息判断连接方式。', ethernet: '网线：表示设备通过有线网络连接；除非来源是设备协议，否则不代表已确认具体交换机端口。', wifi: 'Wi-Fi：表示设备通过无线网络连接；不一定能确认具体接入点。', logical: '逻辑连接：只用于整理设备归属，不代表真实网线、无线接入点或交换机端口。', virtual: '虚拟连接：用于 Internet、虚拟机、容器或其他没有对应实体网线的关系。'};
      const source = connection?.user_confirmed || connection?.Confirmed ? '这条关系由用户确认。' : connection ? '这条关系由系统推测。' : '保存后会作为用户确认的关系。';
      document.querySelector('#connectionHelp').textContent = `${descriptions[connectionSelect.value]} ${source}`;
    };
    connectionSelect.onchange = explain;
    explain();
  }
};

const originalSaveDetail = saveDetail;
saveDetail = async function (device) {
  const important = document.querySelector('#editImportant')?.checked || false;
  const presenceMode = document.querySelector('#editPresence')?.value || 'normal';
  try {
    await api(`/api/devices/${device.id}`, {method: 'PATCH', body: JSON.stringify({is_important: important, presence_mode: presenceMode})});
  } catch (error) {
    toast(error.message);
    return;
  }
  return originalSaveDetail(device);
};

async function loadDeviceHistory(id, hours) {
  const summary = document.querySelector('#historySummary');
  const chart = document.querySelector('#historyChart');
  if (!summary || !chart) return;
  try {
    const data = await api(`/api/devices/${id}/history?hours=${hours}`);
    summary.innerHTML = `<b>${data.uptime_percent.toFixed(1)}%</b><span>在线率</span><b>${data.average_latency_ms ? data.average_latency_ms.toFixed(1) + ' ms' : '—'}</b><span>平均延迟</span><b>${data.samples}</b><span>检查次数</span>`;
    if (!data.points.length) {
      chart.innerHTML = '<p class="muted">还没有历史数据，完成几次扫描后这里会出现曲线。</p>';
      return;
    }
    const values = data.points.map(point => point.status === 'online' && point.latency_ms > 0 ? point.latency_ms : null);
    const maximum = Math.max(10, ...values.filter(value => value !== null));
    const width = 320, height = 90, step = values.length > 1 ? width / (values.length - 1) : width;
    const segments = [];
    let current = [];
    values.forEach((value, index) => {
      if (value === null) {
        if (current.length) segments.push(current);
        current = [];
      } else {
        current.push(`${(index * step).toFixed(1)},${(height - value / maximum * (height - 12)).toFixed(1)}`);
      }
    });
    if (current.length) segments.push(current);
    const offline = values.map((value, index) => value === null ? `<rect x="${Math.max(0, index * step - 2)}" y="0" width="4" height="${height}" rx="2"/>` : '').join('');
    chart.innerHTML = `<svg viewBox="0 0 ${width} ${height}" preserveAspectRatio="none" role="img" aria-label="设备延迟历史"><g class="history-offline">${offline}</g>${segments.map(points => `<polyline points="${points.join(' ')}"/>`).join('')}</svg><div class="history-legend"><span>绿色：延迟</span><span>红色：未在线</span></div>`;
  } catch (error) {
    summary.textContent = error.message;
    chart.innerHTML = '';
  }
}

const originalNotificationValues = notificationValues;
notificationValues = function () {
  return {
    ...originalNotificationValues(),
    notification_cooldown: document.querySelector('#notifyCooldown')?.value || '15m',
    notification_important_only: document.querySelector('#notifyImportantOnly')?.checked || false,
    automatic_backup_enabled: document.querySelector('#autoBackupEnabled')?.checked || false,
    automatic_backup_interval: document.querySelector('#autoBackupInterval')?.value || '24h',
    automatic_backup_keep: +(document.querySelector('#autoBackupKeep')?.value || 7),
    history_retention_days: +(document.querySelector('#historyRetentionDays')?.value || 30)
  };
};

const originalEnsureNotificationUI = ensureNotificationUI;
ensureNotificationUI = function () {
  originalEnsureNotificationUI();
  if (!document.querySelector('#notifyCooldown')) {
    document.querySelector('#notificationResult').insertAdjacentHTML('beforebegin', `<div class="formgrid notification-controls"><label>同类消息冷却时间<select id="notifyCooldown"><option value="0s">不限制</option><option value="5m">5 分钟</option><option value="15m">15 分钟</option><option value="1h">1 小时</option><option value="6h">6 小时</option></select></label><label class="check notification-rule"><input id="notifyImportantOnly" type="checkbox" checked disabled>离线与恢复只提醒长期在线设备</label></div>`);
  }
};

const originalOpenSettings = openSettings;
openSettings = async function () {
  await originalOpenSettings();
  document.querySelector('#notifyCooldown').value = settings.notification_cooldown || '15m';
  document.querySelector('#notifyImportantOnly').checked = true;
  document.querySelector('#autoBackupEnabled').checked = settings.automatic_backup_enabled === 'true';
  document.querySelector('#autoBackupInterval').value = settings.automatic_backup_interval || '24h';
  document.querySelector('#autoBackupKeep').value = settings.automatic_backup_keep || 7;
  document.querySelector('#historyRetentionDays').value = settings.history_retention_days || 30;
  loadMaintenanceStatus();
  document.querySelector('#setCIDR').placeholder = '多个网段用逗号分隔，例如 192.168.1.0/24,10.0.0.0/24';
  loadScanDiagnostics();
};

function ensureMaintenanceUI() {
  const pane = document.querySelector('#backupPane');
  if (!pane || document.querySelector('#autoBackupEnabled')) return;
  pane.insertAdjacentHTML('beforeend', `
    <hr><h3>服务器自动备份</h3>
    <p class="muted">备份保存在 MeowTopo 数据目录的 backups 文件夹，不需要额外程序。</p>
    <div class="formgrid">
      <label class="check"><input id="autoBackupEnabled" type="checkbox">启用自动备份</label>
      <label>备份间隔<select id="autoBackupInterval"><option value="12h">每 12 小时</option><option value="24h">每天</option><option value="72h">每 3 天</option><option value="168h">每周</option></select></label>
      <label>保留备份份数<input id="autoBackupKeep" type="number" min="1" max="30" value="7"></label>
      <label>历史记录保留天数<input id="historyRetentionDays" type="number" min="7" max="365" value="30"></label>
    </div>
    <button type="button" id="backupNow">立即在服务器创建一份</button>
    <p id="maintenanceStatus" class="muted">正在读取备份状态…</p>`);
  document.querySelector('#backupNow').onclick = async () => {
    const result = document.querySelector('#maintenanceStatus');
    result.textContent = '正在创建备份…';
    try {
      await api('/api/maintenance/backup', {method: 'POST'});
      toast('服务器备份已创建');
      loadMaintenanceStatus();
    } catch (error) { result.textContent = error.message; }
  };
}

function ensureQualityTools() {
  const settingsMain = document.querySelector('#settingsMain');
  if (settingsMain && !document.querySelector('#scanDiagnostics')) {
    settingsMain.insertAdjacentHTML('beforeend', `<section class="settings-group quality-tools"><div class="settings-group-title"><b>扫描情况</b><small>了解为什么有些设备可能没有被发现</small></div><div id="scanDiagnostics" class="scan-diagnostics">打开设置后会读取最近一次扫描情况。</div><button type="button" id="refreshDiagnostics">重新检查</button></section>`);
    document.querySelector('#refreshDiagnostics').onclick = loadScanDiagnostics;
  }
  const toolbar = document.querySelector('#managerDialog .manager-toolbar');
  if (toolbar && !document.querySelector('#deviceExport')) {
    toolbar.insertAdjacentHTML('beforeend', `<div id="deviceExport" class="export-actions"><span>导出设备清单</span><a class="button" href="/api/devices/export?format=csv">CSV</a><a class="button" href="/api/devices/export?format=json">JSON</a></div>`);
  }
}

async function loadScanDiagnostics() {
  const target = document.querySelector('#scanDiagnostics');
  if (!target) return;
  target.textContent = '正在检查…';
  try {
    const data = await api('/api/scan/diagnostics');
    const latest = data.latest || {};
    const summary = latest.total_addresses ? `最近扫描 ${latest.scanned_addresses}/${latest.total_addresses} 个地址，发现 ${latest.found_devices} 台设备。` : '还没有扫描记录。';
    const warnings = data.warnings?.length ? `<ul>${data.warnings.map(item => `<li>${esc(item)}</li>`).join('')}</ul>` : '<p class="diagnostic-ok">没有发现明显的扫描环境问题。</p>';
    target.innerHTML = `<p><b>${esc(summary)}</b></p><p>网段：${esc(data.configured_cidrs || '未设置')} · 网卡：${esc(data.interface || '自动选择')}</p>${warnings}`;
  } catch (error) { target.textContent = error.message; }
}

function shellIcon(name) {
  const paths = {
    home: '<path d="M3 11.5 12 4l9 7.5M5.5 10v10h13V10M9 20v-6h6v6"/>',
    devices: '<rect x="3" y="4" width="18" height="13" rx="3"/><path d="M8 21h8m-4-4v4M7 9h.01m4 0h6"/>',
    activity: '<path d="M4 19V9m5 10V5m5 14v-7m5 7V3"/>',
    settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1a1.7 1.7 0 0 0 1.9.3A1.7 1.7 0 0 0 10 3V2.8h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z"/>',
    layout: '<circle cx="6" cy="6" r="2"/><circle cx="18" cy="6" r="2"/><circle cx="12" cy="18" r="2"/><path d="m7.7 7.1 3.2 8.8m5.4-8.8-3.2 8.8M8 6h8"/>',
    fit: '<path d="M8 3H3v5m13-5h5v5M8 21H3v-5m13 5h5v-5"/>'
  };
  return `<svg viewBox="0 0 24 24" aria-hidden="true">${paths[name]}</svg>`;
}

function ensureDashboardChrome() {
  if (document.querySelector('#appSidebar')) return;
  document.body.classList.add('dashboard-shell');
  const header = document.querySelector('.topbar');
  const main = document.querySelector('main');
  const sidebar = document.createElement('aside');
  sidebar.id = 'appSidebar';
  sidebar.className = 'app-sidebar';
  sidebar.innerHTML = '<nav class="sidebar-nav"></nav><div class="sidebar-foot"><span class="sidebar-pulse"></span><span>本地运行中</span></div>';
  document.querySelector('#app').insertBefore(sidebar, header);

  const brand = document.querySelector('#homeBtn');
  sidebar.insertBefore(brand, sidebar.firstChild);
  brand.onclick = () => cy?.fit(undefined, 60);
  const nav = sidebar.querySelector('.sidebar-nav');
  const entries = [
    [document.querySelector('#manageBtn'), 'devices', '设备管理'],
    [document.querySelector('#activityBtn'), 'activity', '运行记录'],
    [document.querySelector('#settingsBtn'), 'settings', '系统设置']
  ];
  entries.forEach(([button, icon, label]) => {
    button.classList.remove('primary');
    button.classList.add('nav-button');
    button.innerHTML = `${shellIcon(icon)}<span>${label}</span>`;
    nav.appendChild(button);
  });
  nav.insertAdjacentHTML('afterbegin', `<button type="button" class="nav-button active" id="overviewNav">${shellIcon('home')}<span>拓扑总览</span></button>`);
  document.querySelector('#overviewNav').onclick = () => cy?.fit(undefined, 60);

  header.insertAdjacentHTML('afterbegin', '<div class="page-heading"><small>家庭网络中心</small><strong>网络拓扑</strong></div>');
  main.querySelector('#canvasPage').insertAdjacentHTML('beforeend', `
    <section class="canvas-context" aria-label="拓扑状态">
      <span class="context-kicker"><i></i> LIVE TOPOLOGY</span>
      <strong>家庭网络拓扑</strong>
      <small id="networkHealthText">正在读取网络状态…</small>
    </section>`);
  const toolbar = document.createElement('div');
  toolbar.className = 'canvas-toolbar';
  main.querySelector('#canvasPage').appendChild(toolbar);
  const layout = document.querySelector('#layoutBtn'), fit = document.querySelector('#fitBtn');
  layout.innerHTML = `${shellIcon('layout')}<span>整理布局</span>`;
  fit.innerHTML = `${shellIcon('fit')}<span>适应画布</span>`;
  toolbar.append(layout, fit);
  updateDashboardHealth();
}

function updateDashboardHealth() {
  const target = document.querySelector('#networkHealthText');
  if (!target) return;
  const online = devices.filter(device => device.status === 'online').length;
  const attention = devices.filter(device => device.is_important && ['offline', 'suspected_offline'].includes(device.status)).length;
  target.textContent = devices.length ? `${online} 台在线${attention ? ` · ${attention} 台长期在线设备已离线` : ' · 长期在线设备状态良好'}` : '等待第一次网络扫描';
  target.classList.toggle('has-warning', attention > 0);
}

const originalDashboardRefresh = refresh;
refresh = async function (...args) {
  const result = await originalDashboardRefresh(...args);
  updateDashboardHealth();
  return result;
};

const originalActivityLoad = loadActivity;
loadActivity = async function () {
  await originalActivityLoad();
  const events = document.querySelectorAll('#statusEventList .activity-item');
  const scans = document.querySelectorAll('#scanHistoryList .activity-item');
  const eventCount = document.querySelector('#activityEventCount');
  const scanCount = document.querySelector('#activityScanCount');
  const latest = document.querySelector('#activityLatestTime');
  if (eventCount) eventCount.textContent = events.length;
  if (scanCount) scanCount.textContent = scans.length;
  const latestText = events[0]?.querySelector('small')?.textContent?.split(' · ').pop() || scans[0]?.querySelector('small')?.textContent?.split(' · ')[0] || '暂无';
  if (latest) latest.textContent = latestText;
};

const originalManagerRender = renderManager;
renderManager = function () {
  originalManagerRender();
  document.querySelector('#managerDialog .batch-bar')?.classList.toggle('has-selection', selectedDevices.size > 0);
  document.querySelectorAll('#managerList .manager-device').forEach(row => {
    const actions = row.querySelector('.manager-device-actions');
    const id = +(actions?.querySelector('[data-id]')?.dataset.id || 0);
    const device = devices.find(item => item.id === id);
    if (!actions || !device) return;
    if (device.is_important) row.querySelector('.manager-device-main b')?.insertAdjacentHTML('beforeend', '<span class="badge attention">长期在线</span>');
    if (device.presence_mode === 'occasional') row.querySelector('.manager-device-main b')?.insertAdjacentHTML('beforeend', '<span class="badge occasional">偶尔在线</span>');
    if (device.is_flapping) row.querySelector('.manager-device-main b')?.insertAdjacentHTML('beforeend', '<span class="badge unstable">状态不稳定</span>');
    actions.insertAdjacentHTML('afterbegin', `<button data-action="attention" data-id="${id}">${device.is_important ? '取消长期在线' : '设为长期在线'}</button>`);
    actions.querySelector('[data-action="attention"]').onclick = event => managerAction(event.currentTarget);
    row.onclick = event => {
      if (event.target.closest('button,input,label')) return;
      document.querySelector('#managerDialog').close();
      openDetail(id);
    };
  });
};

const originalManagerOpen = openManager;
openManager = function () {
  originalManagerOpen();
  document.querySelector('#managerDialog').scrollTop = 0;
};

const originalManagerAction = managerAction;
managerAction = async function (button) {
  if (button.dataset.action !== 'attention') return originalManagerAction(button);
  const id = +button.dataset.id;
  const device = devices.find(item => item.id === id);
  if (!device) return;
  const wasImportant = device.is_important;
  try {
    await api(`/api/devices/${id}`, {method: 'PATCH', body: JSON.stringify({is_important: !wasImportant})});
    await refresh();
    renderManager();
    toast(wasImportant ? '已取消长期在线标记' : '已设为长期在线设备');
  } catch (error) { toast(error.message); }
};

async function loadMaintenanceStatus() {
  const result = document.querySelector('#maintenanceStatus');
  if (!result) return;
  try {
    const status = await api('/api/maintenance');
    if (!status.backup_count) { result.textContent = '服务器上还没有自动备份。'; return; }
    const last = new Date(status.backups[0].created_at).toLocaleString();
    result.textContent = `服务器上有 ${status.backup_count} 份备份，最近一份：${last}`;
  } catch (error) { result.textContent = error.message; }
}

const originalBind = bind;
bind = function () {
  ensureMaintenanceUI();
  ensureQualityTools();
  originalBind();
  ensureDashboardChrome();
  document.querySelector('#aboutTab').onclick = async () => {
    showPane('about');
    const result = await api('/api/version');
    const value = result.version || 'dev';
    document.querySelector('#versionText').textContent = value.startsWith('dev-') ? `开发版 · ${value.slice(4)}` : value === 'dev' ? '开发版 · 本地构建' : `版本 ${value}`;
  };
  const overview = document.querySelector('#overviewNav');
  const navPages = [
    [document.querySelector('#manageBtn'), document.querySelector('#managerClose')],
    [document.querySelector('#activityBtn'), document.querySelector('#activityClose')],
    [document.querySelector('#settingsBtn'), document.querySelector('#settingsForm button[value="cancel"]')],
    [document.querySelector('#accountBtn'), document.querySelector('#accountClose')]
  ];
  const workspaceDialogs = ['managerDialog', 'activityDialog', 'settingsDialog', 'accountDialog'];
  const closeWorkspacePages = () => workspaceDialogs.forEach(id => {
    const dialog = document.querySelector(`#${id}`);
    if (dialog?.open) dialog.close();
  });
  const activateNav = button => {
    document.querySelectorAll('.sidebar-nav .nav-button').forEach(item => item.classList.toggle('active', item === button));
    document.body.classList.toggle('workspace-open', button !== overview);
  };
  navPages.forEach(([open, close]) => {
    if (open) {
      const action = open.onclick;
      open.onclick = async event => {
        closeWorkspacePages();
        activateNav(open);
        const result = await action?.call(open, event);
        if (workspaceDialogs.some(id => document.querySelector(`#${id}`)?.open)) activateNav(open);
        return result;
      };
    }
    if (close) {
      const action = close.onclick;
      close.onclick = event => { activateNav(overview); return action?.call(close, event); };
    }
  });
  workspaceDialogs.forEach(id => document.querySelector(`#${id}`)?.addEventListener('close', () => {
    setTimeout(() => {
      if (!workspaceDialogs.some(dialogID => document.querySelector(`#${dialogID}`)?.open)) activateNav(overview);
    }, 0);
  }));
  if (overview) overview.onclick = () => { closeWorkspacePages(); activateNav(overview); cy?.fit(undefined, 60); };
  document.querySelectorAll('[data-activity-pane]').forEach(button => button.onclick = () => {
    document.querySelectorAll('[data-activity-pane]').forEach(item => item.classList.toggle('active', item === button));
    document.querySelectorAll('[data-activity-content]').forEach(pane => pane.classList.toggle('hidden', pane.dataset.activityContent !== button.dataset.activityPane));
  });
  const createForm = document.querySelector('#accountCreateForm');
  const permissionGrid = createForm?.querySelector('.permission-grid');
  if (permissionGrid && !document.querySelector('#accountRolePreset')) {
    permissionGrid.insertAdjacentHTML('beforebegin', `<label class="role-preset">使用权限<select id="accountRolePreset"><option value="view">只查看（适合访客）</option><option value="family">家庭成员（可整理设备）</option><option value="maintain">协助管理（可扫描和设置）</option><option value="admin">管理员（全部权限）</option></select><small>先选择常用权限，需要时再在下方微调。</small></label>`);
    document.querySelector('#accountRolePreset').onchange = event => {
      const role = event.target.value;
      document.querySelector('#newAdmin').checked = role === 'admin';
      document.querySelector('#newCanEdit').checked = ['family', 'maintain', 'admin'].includes(role);
      document.querySelector('#newCanScan').checked = ['maintain', 'admin'].includes(role);
      document.querySelector('#newCanSettings').checked = ['maintain', 'admin'].includes(role);
      document.querySelectorAll('.permission-grid input').forEach(input => input.disabled = role === 'admin');
    };
  }
  [document.querySelector('#wizardLater'), document.querySelector('#settingsForm button[value="cancel"]'), document.querySelector('#manualForm button[value="cancel"]')].filter(Boolean).forEach(button => {
    button.type = 'button';
    button.onclick = () => {
      const form = button.closest('form');
      form?.reset();
      button.closest('dialog')?.close('cancel');
    };
  });
  const restore = document.querySelector('#restoreFile');
  if (!restore) return;
  restore.onchange = async event => {
    const file = event.target.files[0];
    if (!file || !confirm('恢复会替换当前数据库，是否继续？')) return;
    try {
      const response = await fetch('/api/restore', {method: 'POST', headers: {'Content-Type': 'application/zip', 'X-CSRF-Token': csrfToken}, body: file});
      if (!response.ok) throw new Error((await response.json()).error.message);
      toast('恢复完成，请重新登录');
      setTimeout(() => location.reload(), 1000);
    } catch (error) {
      toast(error.message);
    }
  };
};
