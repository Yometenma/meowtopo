/* Feature extensions kept separate from the compact core UI bundle. */
const originalOpenDetail = openDetail;
const originalNodeDisplay = nodeDisplay;
nodeDisplay = device => `${device.is_important ? '★ ' : ''}${originalNodeDisplay(device)}`;
openDetail = function (id) {
  originalOpenDetail(id);
  const device = devices.find(item => item.id === id);
  const deviceConnections = connections.filter(item => (item.TargetID || item.target_device_id) === id);
  const editTitle = [...document.querySelectorAll('#detail h3')].find(item => item.textContent === '编辑');
  const detailTerms = [...document.querySelectorAll('#detail .detail-grid dt')];
  const sourceTerm = detailTerms.find(item => item.textContent === '识别依据');
  const sourceLabels = {hostname: '主机名', ports: '开放端口', mdns: 'mDNS 服务', ssdp: 'SSDP/UPnP', dhcp: 'DHCP', mac: 'MAC 厂商'};
  if (sourceTerm && device?.identification_source) {
    const sources = device.identification_source.split('+').map(source => sourceLabels[source] || source);
    sourceTerm.nextElementSibling.textContent = `${sources.join(' + ')} · ${Math.round((device.identification_confidence || 0) * 100)}%`;
  }
  if (editTitle && device?.identification_evidence?.length) {
    editTitle.insertAdjacentHTML('beforebegin', `
      <section class="identification-card">
        <h3>为什么这样识别</h3>
        <ul>${device.identification_evidence.map(item => `<li>${esc(item)}</li>`).join('')}</ul>
        <small>自动识别仅供参考，你手工设置的类型始终优先。</small>
      </section>`);
  }
  if (editTitle && device?.user_device_type && device.user_device_type !== device.auto_device_type) {
    editTitle.insertAdjacentHTML('beforebegin', `<p class="device-correction">你已将自动识别的“${esc(typeLabel(device.auto_device_type))}”修正为“${esc(typeLabel(device.user_device_type))}”。喵拓会保留你的选择。</p>`);
  }
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
  const parentSelect = document.querySelector('#editParent');
  if (parentSelect) {
    parentSelect.value = '';
    parentSelect.closest('label').childNodes[0].textContent = '添加上级设备';
    const connectionTypeSelect = document.querySelector('#editConn');
    const connectionPortInput = document.querySelector('#editPort');
    if (connectionTypeSelect) connectionTypeSelect.value = 'unknown';
    if (connectionPortInput) {
      connectionPortInput.value = '';
      connectionPortInput.closest('label').childNodes[0].textContent = '端口或说明';
    }
    const connectionList = deviceConnections.map(connection => {
      const parentID = connection.SourceID || connection.source_device_id;
      const parent = devices.find(item => item.id === parentID);
      const type = connection.Type || connection.connection_type || 'unknown';
      const port = connection.Port || connection.port_label || '';
      const confirmed = connection.Confirmed ?? connection.user_confirmed;
      const sourceType = connection.SourceType || connection.source_type || (confirmed ? 'manual' : 'inferred');
      const confidence = Number(connection.Confidence ?? connection.confidence ?? 0);
      const evidence = confirmed ? '你确认的连接 · 高可信度' : `${connectionSourceLabel(sourceType)} · ${confidence ? `可信度 ${Math.round(confidence * 100)}%` : '可信度未知'}`;
      return `<div class="connection-row"><div><b>${esc(parent ? nameOf(parent) : `设备 ${parentID}`)}</b><span>${esc(connectionTypeLabel(type))}${port ? ` · ${esc(port)}` : ''}</span><small>${esc(evidence)}</small></div><button type="button" data-remove-connection="${connection.ID || connection.id}">移除</button></div>`;
    }).join('');
    parentSelect.closest('label').insertAdjacentHTML('beforebegin', `<section class="connection-list"><h3>当前连接 <span>${deviceConnections.length}</span></h3>${connectionList || '<p class="muted">还没有上级连接。</p>'}</section>`);
    document.querySelectorAll('[data-remove-connection]').forEach(button => button.onclick = async () => {
      if (!confirm('确定移除这条连接吗？其他连接不会受影响。')) return;
      try {
        await api(`/api/devices/${id}/connections/${button.dataset.removeConnection}`, {method: 'DELETE'});
        await refresh();
        openDetail(id);
        toast('连接已移除');
      } catch (error) {
        toast(error.message);
      }
    });
  }
  const connectionSelect = document.querySelector('#editConn');
  if (connectionSelect) {
    connectionSelect.insertAdjacentHTML('afterend', '<small id="connectionHelp" class="connection-help"></small>');
    const explain = () => {
      const descriptions = {unknown: '未知连接：还没有足够信息判断连接方式。', ethernet: '网线：表示设备通过有线网络连接；除非来源是设备协议，否则不代表已确认具体交换机端口。', wifi: 'Wi-Fi：表示设备通过无线网络连接；不一定能确认具体接入点。', logical: '逻辑连接：只用于整理设备归属，不代表真实网线、无线接入点或交换机端口。', virtual: '虚拟连接：用于 Internet、虚拟机、容器或其他没有对应实体网线的关系。'};
      document.querySelector('#connectionHelp').textContent = `${descriptions[connectionSelect.value]} 选择上级设备并保存后，会新增一条连接，不会覆盖其他连接。`;
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
    await api(`/api/devices/${device.id}`, {method: 'PATCH', body: JSON.stringify({
      user_name: document.querySelector('#editName').value,
      user_device_type: document.querySelector('#editType').value,
      notes: document.querySelector('#editNotes').value,
      is_new: false,
      is_ignored: document.querySelector('#editIgnored').checked,
      always_show: document.querySelector('#editAlways').checked,
      is_important: important,
      presence_mode: presenceMode
    })});
    const parentID = +(document.querySelector('#editParent')?.value || 0);
    if (parentID) {
      await api(`/api/devices/${device.id}/connections`, {method: 'POST', body: JSON.stringify({
        parent_id: parentID,
        connection_type: document.querySelector('#editConn').value,
        port_label: document.querySelector('#editPort').value
      })});
    }
    await refresh();
    openDetail(device.id);
    toast(parentID ? '设备信息和连接已保存' : '设备信息已保存');
  } catch (error) {
    toast(error.message);
  }
};

function connectionTypeLabel(type) {
  return {unknown: '未知连接', ethernet: '网线', wifi: 'Wi-Fi', logical: '逻辑连接', virtual: '虚拟连接'}[type] || '未知连接';
}

function connectionSourceLabel(source) {
  return {manual: '用户设置', inferred: '系统低可信度推测', lldp: 'LLDP 发现', snmp: 'SNMP 发现', controller: '网络控制器提供'}[source] || '来源未知';
}

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
    fit: '<path d="M8 3H3v5m13-5h5v5M8 21H3v-5m13 5h5v-5"/>',
    select: '<path d="M4 8V4h4m8 0h4v4M4 16v4h4m8 0h4v-4"/><path d="M9 9h6v6H9z"/>'
  };
  return `<svg viewBox="0 0 24 24" aria-hidden="true">${paths[name]}</svg>`;
}

function ensureCanvasHelpUI() {
  if (document.querySelector('#canvasHelpDialog')) return;
  document.body.insertAdjacentHTML('beforeend', `<dialog id="canvasHelpDialog" class="canvas-help-dialog">
    <div class="canvas-help-head"><div><span class="page-kicker">TOPOLOGY GUIDE</span><h2 id="canvasHelpTitle">拓扑操作指南</h2><p>不用切换编辑模式，直接在画布上操作。</p></div><button type="button" id="canvasHelpClose">关闭</button></div>
    <div class="canvas-help-platforms">
      <section><b>Windows</b><dl><dt>移动画布</dt><dd>按住空白处拖动</dd><dt>缩放</dt><dd>滚轮</dd><dt>框选多个</dt><dd>Shift + 拖动空白处</dd><dt>快捷操作</dt><dd>右键设备或连接线</dd><dt>全选</dt><dd>Ctrl + A</dd></dl></section>
      <section><b>macOS</b><dl><dt>移动画布</dt><dd>按住空白处拖动</dd><dt>缩放</dt><dd>触控板捏合或滚动</dd><dt>框选多个</dt><dd>Shift + 拖动空白处</dd><dt>快捷操作</dt><dd>双指点按设备或连接线</dd><dt>全选</dt><dd>⌘ + A</dd></dl></section>
      <section><b>手机与平板</b><dl><dt>移动画布</dt><dd>单指拖动空白处</dd><dt>缩放</dt><dd>双指捏合</dd><dt>选择多个</dt><dd>点“选择”，再逐个点设备</dd><dt>快捷操作</dt><dd>长按设备或连接线</dd><dt>移动一组</dt><dd>拖动任意已选设备</dd></dl></section>
    </div>
    <section class="connection-guide"><h3>连接方式代表什么</h3><div><article><b>网线</b><p>确认设备之间存在有线连接。</p></article><article><b>Wi-Fi</b><p>确认设备通过无线网络接入上级。</p></article><article><b>逻辑连接</b><p>表示流量、路由或服务上的上下级，不宣称真实网线接法。</p></article><article><b>虚拟连接</b><p>用于虚拟机、容器、虚拟网桥等软件关系。</p></article><article><b>未知连接</b><p>知道设备有关联，但目前不能确定连接方式。</p></article><article><b>虚线推测</b><p>由喵拓根据有限信息推测，可由你手动确认或修改。</p></article></div></section>
  </dialog>`);
  const dialog = document.querySelector('#canvasHelpDialog');
  dialog.setAttribute('aria-labelledby', 'canvasHelpTitle');
  document.querySelector('#canvasHelpClose').onclick = () => dialog.close();
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
    button.dataset.workspace = icon;
    button.setAttribute('aria-label', label);
    button.classList.remove('primary');
    button.classList.add('nav-button');
    button.innerHTML = `${shellIcon(icon)}<span>${label}</span>`;
    nav.appendChild(button);
  });
  nav.insertAdjacentHTML('afterbegin', `<button type="button" class="nav-button active" id="overviewNav" data-workspace="overview" aria-label="拓扑总览" aria-current="page">${shellIcon('home')}<span>拓扑总览</span></button>`);
  document.querySelector('#overviewNav').onclick = () => cy?.fit(undefined, 60);

  header.insertAdjacentHTML('afterbegin', '<div class="page-heading" aria-live="polite"><small id="pageEyebrow">家庭网络中心</small><strong id="pageTitle">网络拓扑</strong></div>');
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
  toolbar.insertAdjacentHTML('beforeend', '<button type="button" id="canvasHelpBtn" title="查看拓扑操作指南"><b class="help-symbol">?</b><span>使用帮助</span></button>');
  toolbar.insertAdjacentHTML('afterbegin', `<button type="button" id="mobileSelectBtn" class="mobile-select-button${can(permission.edit) ? '' : ' hidden'}" title="选择多个设备">${shellIcon('select')}<span>选择</span></button>`);
  main.querySelector('#canvasPage').insertAdjacentHTML('beforeend', `<div id="selectionBar" class="selection-bar hidden"><b><span id="selectionCount">0</span> 台已选</b><button type="button" data-align="horizontal">横向对齐</button><button type="button" data-align="vertical">纵向对齐</button><button type="button" id="clearSelectionBtn">取消选择</button></div>`);
  document.querySelector('.legend')?.insertAdjacentHTML('beforeend', '<span class="muted selection-hint">Shift + 拖动框选</span>');
  updateDashboardHealth();
  ensureCanvasHelpUI();

  const statusCards = [
    [document.querySelector('#onlineN')?.closest('span'), 'online', '查看在线设备'],
    [document.querySelector('#suspectN')?.closest('span'), 'suspected_offline', '查看疑似离线设备'],
    [document.querySelector('#offlineN')?.closest('span'), 'offline', '查看离线设备'],
    [document.querySelector('#totalN')?.closest('span'), '', '查看全部设备']
  ];
  statusCards.forEach(([card, status, label]) => {
    if (!card) return;
    card.classList.add('status-card');
    card.dataset.status = status;
    card.tabIndex = 0;
    card.setAttribute('role', 'button');
    card.setAttribute('aria-label', label);
  });
}

let quickLinkSource = null;
let mobileSelectionMode = false;

function closeTopologyMenu() {
  document.querySelector('#topologyMenu')?.remove();
}

function updateSelectionBar() {
  const count = cy ? cy.nodes(':selected').length : 0;
  const bar = document.querySelector('#selectionBar');
  if (!bar) return;
  document.querySelector('#selectionCount').textContent = count;
  bar.classList.toggle('hidden', count < 2);
}

function setMobileSelectionMode(enabled) {
  mobileSelectionMode = enabled && can(permission.edit);
  document.querySelector('#mobileSelectBtn')?.classList.toggle('active', mobileSelectionMode);
  if (mobileSelectionMode) toast('逐个点选设备，再拖动任一已选设备即可整组移动');
}

function alignSelectedNodes(axis) {
  if (!cy || !can(permission.edit)) return;
  const selected = cy.nodes(':selected').filter(node => !node.locked());
  if (selected.length < 2) return;
  const values = selected.map(node => axis === 'horizontal' ? node.position('y') : node.position('x'));
  const center = values.reduce((sum, value) => sum + value, 0) / values.length;
  selected.forEach(node => {
    node.position(axis === 'horizontal' ? {x: node.position('x'), y: center} : {x: center, y: node.position('y')});
    savePosition(node);
  });
  toast(axis === 'horizontal' ? '已横向对齐选中设备' : '已纵向对齐选中设备');
}

function installDirectBoxSelection() {
  const container = document.querySelector('#cy');
  if (!container || container.dataset.boxSelectionReady) return;
  container.dataset.boxSelectionReady = 'true';
  container.addEventListener('mousedown', event => {
    if (!event.shiftKey || event.button !== 0 || !cy || !can(permission.edit)) return;
    const containerRect = container.getBoundingClientRect();
    const x = event.clientX - containerRect.left;
    const y = event.clientY - containerRect.top;
    const overNode = cy.nodes(':visible').some(node => {
      const box = node.renderedBoundingBox({includeLabels: true, includeOverlays: true});
      return x >= box.x1 && x <= box.x2 && y >= box.y1 && y <= box.y2;
    });
    if (overNode) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    const page = document.querySelector('#canvasPage');
    const pageRect = page.getBoundingClientRect();
    const startX = event.clientX - pageRect.left;
    const startY = event.clientY - pageRect.top;
    const marquee = document.createElement('div');
    marquee.className = 'selection-marquee';
    page.appendChild(marquee);
    const move = moveEvent => {
      const currentX = moveEvent.clientX - pageRect.left;
      const currentY = moveEvent.clientY - pageRect.top;
      marquee.style.left = `${Math.min(startX, currentX)}px`;
      marquee.style.top = `${Math.min(startY, currentY)}px`;
      marquee.style.width = `${Math.abs(currentX - startX)}px`;
      marquee.style.height = `${Math.abs(currentY - startY)}px`;
    };
    const finish = upEvent => {
      window.removeEventListener('mousemove', move, true);
      window.removeEventListener('mouseup', finish, true);
      const endX = upEvent.clientX - containerRect.left;
      const endY = upEvent.clientY - containerRect.top;
      const left = Math.min(x, endX), right = Math.max(x, endX);
      const top = Math.min(y, endY), bottom = Math.max(y, endY);
      cy.nodes(':visible').forEach(node => {
        const position = node.renderedPosition();
        if (position.x >= left && position.x <= right && position.y >= top && position.y <= bottom) node.select();
      });
      marquee.remove();
      updateSelectionBar();
    };
    move(event);
    window.addEventListener('mousemove', move, true);
    window.addEventListener('mouseup', finish, true);
  }, true);
}

async function createQuickConnection(targetID, connectionType) {
  if (!quickLinkSource || quickLinkSource === targetID) return;
  try {
    await api(`/api/devices/${targetID}/connections`, {method: 'POST', body: JSON.stringify({parent_id: quickLinkSource, connection_type: connectionType, port_label: ''})});
    const source = devices.find(device => device.id === quickLinkSource);
    const target = devices.find(device => device.id === targetID);
    quickLinkSource = null;
    cy.nodes().removeClass('link-source');
    closeTopologyMenu();
    await refresh();
    toast(`已连接：${source ? nameOf(source) : '上级设备'} → ${target ? nameOf(target) : '目标设备'}`);
  } catch (error) { toast(error.message); }
}

function openTopologyMenu(node, renderedPosition) {
  closeTopologyMenu();
  const id = +node.id();
  const editable = can(permission.edit);
  const menu = document.createElement('div');
  menu.id = 'topologyMenu';
  menu.className = 'topology-menu';
  const linkChoices = quickLinkSource && quickLinkSource !== id ? `<div class="topology-menu-title">连接到 ${esc(nameOf(devices.find(device => device.id === id) || {user_name: '此设备'}))}</div><button data-link-type="ethernet">网线连接</button><button data-link-type="wifi">Wi-Fi 连接</button><button data-link-type="logical">逻辑连接</button><button data-link-type="virtual">虚拟连接</button><button data-cancel-link>取消连线</button>` : `${editable ? '<button data-start-link>从此设备开始连线</button>' : ''}<button data-toggle-select>${node.selected() ? '取消选择' : '加入选择'}</button><button data-open-detail>查看详情</button>`;
  menu.innerHTML = linkChoices;
  const canvasRect = document.querySelector('#canvasPage').getBoundingClientRect();
  menu.style.left = `${Math.min(canvasRect.width - 190, Math.max(8, renderedPosition.x + 12))}px`;
  menu.style.top = `${Math.min(canvasRect.height - 230, Math.max(8, renderedPosition.y + 12))}px`;
  document.querySelector('#canvasPage').appendChild(menu);
  menu.querySelector('[data-start-link]')?.addEventListener('click', () => {
    quickLinkSource = id;
    cy.nodes().removeClass('link-source');
    node.addClass('link-source');
    closeTopologyMenu();
    toast(`已选择“${nameOf(devices.find(device => device.id === id))}”作为上级，请右键或长按目标设备`);
  });
  menu.querySelector('[data-toggle-select]')?.addEventListener('click', () => {
    node.selected() ? node.unselect() : node.select();
    closeTopologyMenu();
  });
  menu.querySelector('[data-open-detail]')?.addEventListener('click', () => { closeTopologyMenu(); openDetail(id); });
  menu.querySelector('[data-cancel-link]')?.addEventListener('click', () => { quickLinkSource = null; cy.nodes().removeClass('link-source'); closeTopologyMenu(); });
  menu.querySelectorAll('[data-link-type]').forEach(button => button.addEventListener('click', () => createQuickConnection(id, button.dataset.linkType)));
}

function openConnectionMenu(edge, renderedPosition) {
  closeTopologyMenu();
  const targetID = +(edge.data('target') || 0);
  const connectionID = +(String(edge.id()).replace(/^e/, '') || 0);
  const source = devices.find(device => device.id === +(edge.data('source') || 0));
  const target = devices.find(device => device.id === targetID);
  const menu = document.createElement('div');
  menu.id = 'topologyMenu';
  menu.className = 'topology-menu';
  const sourceType = edge.data('sourceType') || 'inferred';
  const confidence = Number(edge.data('confidence') || 0);
  menu.innerHTML = `<div class="topology-menu-title">${esc(source ? nameOf(source) : '上级设备')} → ${esc(target ? nameOf(target) : '目标设备')}</div><small>${esc(connectionTypeLabel(edge.data('type')))} · ${esc(connectionSourceLabel(sourceType))}${confidence ? ` · ${Math.round(confidence * 100)}%` : ''}</small>${can(permission.edit) ? '<button data-remove-edge>移除这条连接</button>' : ''}`;
  const canvasRect = document.querySelector('#canvasPage').getBoundingClientRect();
  menu.style.left = `${Math.min(canvasRect.width - 210, Math.max(8, renderedPosition.x + 12))}px`;
  menu.style.top = `${Math.min(canvasRect.height - 150, Math.max(8, renderedPosition.y + 12))}px`;
  document.querySelector('#canvasPage').appendChild(menu);
  menu.querySelector('[data-remove-edge]')?.addEventListener('click', async () => {
    if (!confirm('确定移除这条连接吗？设备本身不会被删除。')) return;
    try {
      await api(`/api/devices/${targetID}/connections/${connectionID}`, {method: 'DELETE'});
      closeTopologyMenu();
      await refresh();
      toast('连接已移除');
    } catch (error) { toast(error.message); }
  });
}

const originalInitCyForEditor = initCy;
initCy = function () {
  originalInitCyForEditor();
  if (!cy) return;
  cy.boxSelectionEnabled(false);
  cy.selectionType('additive');
  cy.style()
    .selector('node:selected').style({
      'border-width': 6,
      'border-color': '#35c98a',
      'border-opacity': 1,
      'underlay-color': '#35c98a',
      'underlay-opacity': .34,
      'underlay-padding': 15,
      'opacity': 1,
      'z-index': 20
    })
    .selector('node.link-source').style({'border-width': 6, 'border-color': '#df9a3d', 'underlay-color': '#df9a3d', 'underlay-opacity': .3, 'underlay-padding': 15, 'opacity': 1, 'z-index': 21})
    .update();
  cy.off('tap', 'node');
  cy.on('tap', 'node', event => {
    if (mobileSelectionMode) {
      event.target.selected() ? event.target.unselect() : event.target.select();
      return;
    }
    openDetail(+event.target.id());
  });
  cy.off('dragfree', 'node');
  cy.on('dragfree', 'node', event => {
    const moved = event.target.selected() ? cy.nodes(':selected') : event.target;
    moved.forEach(node => savePosition(node));
  });
  cy.on('cxttap', 'node', event => openTopologyMenu(event.target, event.renderedPosition));
  cy.on('cxttap', 'edge', event => openConnectionMenu(event.target, event.renderedPosition));
  cy.on('select unselect', 'node', updateSelectionBar);
  cy.on('tap pan zoom', event => { if (event.target === cy) closeTopologyMenu(); });
  document.querySelector('#cy').addEventListener('contextmenu', event => event.preventDefault());
  installDirectBoxSelection();
};

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
    row.dataset.status = device.status || 'unknown';
    if (device.is_important) row.querySelector('.manager-device-main b')?.insertAdjacentHTML('beforeend', '<span class="badge attention">长期在线</span>');
    if (device.presence_mode === 'occasional') row.querySelector('.manager-device-main b')?.insertAdjacentHTML('beforeend', '<span class="badge occasional">偶尔在线</span>');
    if (device.is_flapping) row.querySelector('.manager-device-main b')?.insertAdjacentHTML('beforeend', '<span class="badge unstable">状态不稳定</span>');
    if (can(permission.edit)) {
      actions.insertAdjacentHTML('afterbegin', `<button data-action="attention" data-id="${id}">${device.is_important ? '取消长期在线' : '设为长期在线'}</button>`);
      actions.querySelector('[data-action="attention"]').onclick = event => managerAction(event.currentTarget);
      const extraActions = [...actions.querySelectorAll('button:not([data-action="edit"])')];
      if (extraActions.length) {
        const menu = document.createElement('details');
        menu.className = 'manager-actions-menu';
        menu.innerHTML = '<summary aria-label="更多设备操作">更多</summary><div></div>';
        extraActions.forEach(button => menu.querySelector('div').appendChild(button));
        actions.appendChild(menu);
        menu.addEventListener('toggle', () => {
          if (!menu.open) return;
          document.querySelectorAll('.manager-actions-menu[open]').forEach(other => {
            if (other !== menu) other.removeAttribute('open');
          });
        });
      }
    }
    row.onclick = event => {
      if (event.target.closest('button,input,label,details,summary')) return;
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

function ensureVendorDatabaseUI() {
  const pane = document.querySelector('#aboutPane');
  if (!pane || document.querySelector('#vendorDatabaseCard')) return;
  pane.insertAdjacentHTML('beforeend', `
    <section id="vendorDatabaseCard" class="settings-group vendor-database-card">
      <div><b>MAC 厂商资料</b><small id="vendorDatabaseStatus">正在读取状态…</small></div>
      <button type="button" id="vendorDatabaseUpdate">从 IEEE 官方更新</button>
      <p class="muted">资料只保存在本机，用来识别网卡厂商；手机的随机 MAC 不参与厂商判断。</p>
    </section>`);
  document.querySelector('#vendorDatabaseUpdate').onclick = async () => {
    const button = document.querySelector('#vendorDatabaseUpdate');
    button.disabled = true;
    button.textContent = '正在更新…';
    try {
      const status = await api('/api/vendor-database/update', {method: 'POST'});
      showVendorDatabaseStatus(status);
      toast(`厂商资料已更新，共 ${status.entries} 条`);
    } catch (error) {
      toast(error.message);
    } finally {
      button.disabled = false;
      button.textContent = '从 IEEE 官方更新';
    }
  };
}

function showVendorDatabaseStatus(status) {
  const target = document.querySelector('#vendorDatabaseStatus');
  if (!target) return;
  target.textContent = status.available
    ? `已有 ${status.entries} 条资料 · 更新于 ${new Date(status.updated_at).toLocaleString()}`
    : '尚未下载，基础扫描不受影响';
}

async function refreshVendorDatabaseStatus() {
  try {
    showVendorDatabaseStatus(await api('/api/vendor-database'));
  } catch (error) {
    const target = document.querySelector('#vendorDatabaseStatus');
    if (target) target.textContent = error.message;
  }
}

const originalBind = bind;
bind = function () {
  ensureMaintenanceUI();
  ensureQualityTools();
  ensureVendorDatabaseUI();
  originalBind();
  ensureDashboardChrome();
  document.querySelector('#mobileSelectBtn').onclick = () => setMobileSelectionMode(!mobileSelectionMode);
  document.querySelector('#canvasHelpBtn').onclick = () => document.querySelector('#canvasHelpDialog').showModal();
  document.querySelector('#clearSelectionBtn').onclick = () => { cy?.nodes().unselect(); setMobileSelectionMode(false); };
  document.querySelectorAll('#selectionBar [data-align]').forEach(button => button.onclick = () => alignSelectedNodes(button.dataset.align));
  document.addEventListener('keydown', event => {
    if (['INPUT', 'TEXTAREA', 'SELECT'].includes(document.activeElement?.tagName)) return;
    if (event.key === 'Escape') {
      closeTopologyMenu();
      quickLinkSource = null;
      cy?.nodes().removeClass('link-source');
      cy?.nodes().unselect();
      setMobileSelectionMode(false);
    }
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a' && cy && can(permission.edit)) {
      event.preventDefault();
      cy.nodes(':visible').select();
      updateSelectionBar();
    }
    if (event.key === '?' && !event.ctrlKey && !event.metaKey && !event.altKey) document.querySelector('#canvasHelpDialog')?.showModal();
  });
  document.querySelector('#aboutTab').onclick = async () => {
    showPane('about');
    const [result] = await Promise.all([api('/api/version'), refreshVendorDatabaseStatus()]);
    const value = result.version || 'dev';
    document.querySelector('#versionText').textContent = value.startsWith('dev-') ? `开发版 · ${value.slice(4)}` : value === 'dev' ? '开发版 · 本地构建' : `版本 ${value}`;
  };
  const overview = document.querySelector('#overviewNav');
  const workspaceMeta = {
    overview: ['家庭网络中心', '网络拓扑'],
    devices: ['设备与连接', '设备管理'],
    activity: ['扫描与状态变化', '运行记录'],
    settings: ['扫描、通知与数据', '系统设置'],
    account: ['成员与访问权限', '账户管理']
  };
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
  const activateNav = (button, workspace = button?.dataset.workspace || 'overview') => {
    document.querySelectorAll('.sidebar-nav .nav-button').forEach(item => {
      const active = item === button;
      item.classList.toggle('active', active);
      if (active) item.setAttribute('aria-current', 'page');
      else item.removeAttribute('aria-current');
    });
    const [eyebrow, title] = workspaceMeta[workspace] || workspaceMeta.overview;
    document.querySelector('#pageEyebrow').textContent = eyebrow;
    document.querySelector('#pageTitle').textContent = title;
    document.title = `${title} · MeowTopo 喵拓`;
    document.body.dataset.workspace = workspace;
    document.body.classList.toggle('workspace-open', workspace !== 'overview');
    document.querySelector('#accountBtn')?.classList.toggle('active', workspace === 'account');
  };
  navPages.forEach(([open, close]) => {
    if (open) {
      const action = open.onclick;
      open.onclick = async event => {
        closeWorkspacePages();
        activateNav(open, open.dataset.workspace);
        const result = await action?.call(open, event);
        if (workspaceDialogs.some(id => document.querySelector(`#${id}`)?.open)) activateNav(open, open.dataset.workspace);
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
  document.querySelectorAll('.status-card').forEach(card => {
    const openFilteredDevices = () => {
      document.querySelector('#manageBtn').click();
      document.querySelector('#managerStatus').value = card.dataset.status;
      renderManager();
    };
    card.onclick = openFilteredDevices;
    card.onkeydown = event => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        openFilteredDevices();
      }
    };
  });
  const accountButton = document.querySelector('#accountBtn');
  if (accountButton) {
    const action = accountButton.onclick;
    accountButton.onclick = async event => {
      closeWorkspacePages();
      activateNav(null, 'account');
      return action?.call(accountButton, event);
    };
  }
  document.querySelector('#managerClose').textContent = '返回拓扑';
  document.querySelector('#accountClose').textContent = '返回拓扑';
  const pageDialogs = [
    ['managerDialog', '设备管理'],
    ['activityDialog', '运行记录'],
    ['settingsDialog', '系统设置'],
    ['accountDialog', '账户管理']
  ];
  pageDialogs.forEach(([id, label]) => {
    const dialog = document.querySelector(`#${id}`);
    const heading = dialog?.querySelector('h2');
    if (!dialog || !heading) return;
    heading.id ||= `${id}Title`;
    dialog.setAttribute('aria-labelledby', heading.id);
    dialog.setAttribute('aria-label', label);
  });
  document.querySelectorAll('#settingsDialog .tabs, #activityDialog .activity-tabs').forEach(tablist => {
    tablist.setAttribute('role', 'tablist');
    tablist.querySelectorAll('button').forEach(button => {
      button.setAttribute('role', 'tab');
      button.setAttribute('aria-selected', String(button.classList.contains('active')));
      button.addEventListener('click', () => {
        tablist.querySelectorAll('button').forEach(item => item.setAttribute('aria-selected', String(item === button)));
      });
    });
  });
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
