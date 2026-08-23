/* Feature extensions kept separate from the compact core UI bundle. */
const originalOpenDetail = openDetail;
const originalNodeDisplay = nodeDisplay;
nodeDisplay = device => `${device.is_important ? '★ ' : ''}${originalNodeDisplay(device)}`;
openDetail = function (id) {
  originalOpenDetail(id);
  const device = devices.find(item => item.id === id);
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
    actions.insertAdjacentHTML('beforebegin', `<label class="check"><input id="editImportant" type="checkbox" ${device?.is_important ? 'checked' : ''}>重要设备（可用于只推送重要设备）</label>`);
  }
};

const originalSaveDetail = saveDetail;
saveDetail = async function (device) {
  const important = document.querySelector('#editImportant')?.checked || false;
  try {
    await api(`/api/devices/${device.id}`, {method: 'PATCH', body: JSON.stringify({is_important: important})});
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
  return {...originalNotificationValues(), notification_cooldown: document.querySelector('#notifyCooldown')?.value || '15m', notification_important_only: document.querySelector('#notifyImportantOnly')?.checked || false};
};

const originalEnsureNotificationUI = ensureNotificationUI;
ensureNotificationUI = function () {
  originalEnsureNotificationUI();
  if (!document.querySelector('#notifyCooldown')) {
    document.querySelector('#notificationResult').insertAdjacentHTML('beforebegin', `<div class="formgrid notification-controls"><label>同类消息冷却时间<select id="notifyCooldown"><option value="0s">不限制</option><option value="5m">5 分钟</option><option value="15m">15 分钟</option><option value="1h">1 小时</option><option value="6h">6 小时</option></select></label><label class="check"><input id="notifyImportantOnly" type="checkbox">只推送标记为重要的设备</label></div>`);
  }
};

const originalOpenSettings = openSettings;
openSettings = async function () {
  await originalOpenSettings();
  document.querySelector('#notifyCooldown').value = settings.notification_cooldown || '15m';
  document.querySelector('#notifyImportantOnly').checked = settings.notification_important_only === 'true';
  document.querySelector('#setCIDR').placeholder = '多个网段用逗号分隔，例如 192.168.1.0/24,10.0.0.0/24';
};

const originalBind = bind;
bind = function () {
  originalBind();
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
