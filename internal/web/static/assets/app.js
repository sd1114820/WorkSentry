const connectionStatus = document.getElementById('connectionStatus');
const timezoneValue = document.getElementById('timezoneValue');
const updatePolicyValue = document.getElementById('updatePolicyValue');
const adminNameValue = document.getElementById('adminName');
const authModal = document.getElementById('authModal');
const settingsWarning = document.getElementById('settingsWarning');
const settingsWarningList = document.getElementById('settingsWarningList');

const sections = document.querySelectorAll('.section');
const navItems = document.querySelectorAll('.nav-item');

let authToken = localStorage.getItem('adminToken') || '';
let adminName = localStorage.getItem('adminName') || '';
let settingsCache = null;
let liveItems = [];
let liveMap = {};
let liveTimer = null;
let wsClient = null;

let editingRuleId = null;
let editingAdjustmentId = null;
let editingIncidentId = null;
let editingDepartmentId = null;
let editingEmployeeId = null;

let departments = [];
let employees = [];
let rulesCache = [];
let adjustmentsCache = [];
let incidentsCache = [];

function switchSection(target) {
  navItems.forEach((item) => {
    item.classList.toggle('is-active', item.dataset.target === target);
  });
  const targetId = 'section-' + target;
  sections.forEach((section) => {
    section.classList.toggle('is-active', section.id === targetId);
  });
}

navItems.forEach((item) => {
  item.addEventListener('click', () => {
    switchSection(item.dataset.target);
  });
});

function showLogin() {
  authModal.classList.add('is-active');
}

function hideLogin() {
  authModal.classList.remove('is-active');
}

function setAuth(token, name) {
  authToken = token || '';
  adminName = name || '';
  if (authToken) {
    localStorage.setItem('adminToken', authToken);
    localStorage.setItem('adminName', adminName);
    adminNameValue.textContent = adminName || '管理员';
    hideLogin();
  } else {
    localStorage.removeItem('adminToken');
    localStorage.removeItem('adminName');
    adminNameValue.textContent = '未登录';
    showLogin();
  }
}

async function fetchJSON(url, options) {
  const opts = options ? { ...options } : {};
  opts.headers = opts.headers || {};
  if (authToken) {
    opts.headers.Authorization = 'Bearer ' + authToken;
  }
  const resp = await fetch(url, opts);
  if (resp.status === 401) {
    setAuth('', '');
    throw new Error('请先登录');
  }
  if (!resp.ok) {
    const data = await resp.json().catch(() => ({}));
    throw new Error(data.message || '请求失败');
  }
  return resp.json();
}

async function fetchBlob(url, options) {
  const opts = options ? { ...options } : {};
  opts.headers = opts.headers || {};
  if (authToken) {
    opts.headers.Authorization = 'Bearer ' + authToken;
  }
  const resp = await fetch(url, opts);
  if (resp.status === 401) {
    setAuth('', '');
    throw new Error('请先登录');
  }
  if (!resp.ok) {
    const data = await resp.json().catch(() => ({}));
    throw new Error(data.message || '请求失败');
  }
  return resp.blob();
}

function setStatus(message, element) {
  if (!element) return;
  element.textContent = message;
  setTimeout(() => {
    element.textContent = '';
  }, 3000);
}

function padNumber(value) {
  return String(value).padStart(2, '0');
}

function formatCountdown(seconds) {
  if (seconds === null || seconds === undefined) return '';
  const total = Math.max(0, Math.floor(seconds));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const remain = total % 60;
  if (hours > 0) {
    return padNumber(hours) + ':' + padNumber(minutes) + ':' + padNumber(remain);
  }
  return padNumber(minutes) + ':' + padNumber(remain);
}

function formatLocalDate(value) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return date.toISOString().slice(0, 10);
}

function formatDateTimeLocal(value) {
  if (!value) return '';
  const normalized = value.includes('T') ? value : value.replace(' ', 'T');
  const date = new Date(normalized);
  if (Number.isNaN(date.getTime())) return '';
  const offset = date.getTimezoneOffset() * 60000;
  return new Date(date - offset).toISOString().slice(0, 16);
}

function buildMetaText(lastSeen, remaining) {
  let text = '上次：' + (lastSeen || '-');
  if (remaining !== null && remaining !== undefined) {
    text += ' · 延迟倒计时 ' + formatCountdown(remaining);
  }
  return text;
}

function setDefaultDates() {
  const today = new Date();
  const dateValue = today.toISOString().slice(0, 10);
  ['reportDate', 'timelineDate', 'offlineDate', 'auditDate'].forEach((id) => {
    const input = document.getElementById(id);
    if (input && !input.value) {
      input.value = dateValue;
    }
  });
}

async function login() {
  const username = document.getElementById('loginUsername').value.trim();
  const password = document.getElementById('loginPassword').value.trim();
  if (!username || !password) {
    setStatus('请输入账号密码', document.getElementById('loginStatus'));
    return;
  }
  try {
    const data = await fetchJSON('/api/v1/admin/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: username, password: password }),
    });
    setAuth(data.token, data.displayName || '管理员');
    await initApp();
  } catch (error) {
    setStatus(error.message, document.getElementById('loginStatus'));
  }
}

function logout() {
  if (wsClient) {
    wsClient.close();
    wsClient = null;
  }
  if (liveTimer) {
    clearInterval(liveTimer);
    liveTimer = null;
  }
  setAuth('', '');
  connectionStatus.textContent = '未连接';
}

function collectSettingsWarnings() {
  const idle = Number(document.getElementById('idleThreshold').value || 0);
  const heartbeat = Number(document.getElementById('heartbeatInterval').value || 0);
  const offline = Number(document.getElementById('offlineThreshold').value || 0);
  const fish = Number(document.getElementById('fishRatioWarn').value || 0);
  const warnings = [];

  if (!idle || idle <= 0) {
    warnings.push('闲置阈值需要大于 0 秒。');
  }
  if (!heartbeat || heartbeat <= 0) {
    warnings.push('心跳间隔需要大于 0 秒。');
  }
  if (!offline || offline <= 0) {
    warnings.push('离线阈值需要大于 0 秒。');
  }

  if (offline > 0 && heartbeat > 0 && offline <= heartbeat) {
    warnings.push('离线阈值应大于心跳间隔，否则可能出现“丢一个心跳就离线”的误判。');
  }

  if (offline > 0 && idle > 0 && offline <= idle) {
    warnings.push('离线阈值应大于闲置阈值，避免“离开未生效就离线”的体验。');
  }

  if (Number.isFinite(fish) && (fish < 0 || fish > 100)) {
    warnings.push('摸鱼比例阈值建议在 0 到 100 之间。');
  }

  return warnings;
}

function renderSettingsWarnings() {
  const warnings = collectSettingsWarnings();
  if (settingsWarning && settingsWarningList) {
    settingsWarningList.innerHTML = warnings.map((item) => '<li>' + item + '</li>').join('');
    settingsWarning.classList.toggle('is-hidden', warnings.length === 0);
  }
  const saveButton = document.getElementById('saveSettings');
  if (saveButton) {
    saveButton.disabled = warnings.length > 0;
  }
  return warnings;
}

async function loadSettings() {
  try {
    const data = await fetchJSON('/api/v1/admin/settings');
    settingsCache = data;
    document.getElementById('idleThreshold').value = data.idleThresholdSeconds;
    document.getElementById('heartbeatInterval').value = data.heartbeatIntervalSeconds;
    document.getElementById('offlineThreshold').value = data.offlineThresholdSeconds;
    document.getElementById('fishRatioWarn').value = data.fishRatioWarnPercent;
    document.getElementById('updatePolicy').value = data.updatePolicy;
    document.getElementById('latestVersion').value = data.latestVersion || '';
    document.getElementById('updateUrl').value = data.updateUrl || '';
    updatePolicyValue.textContent = data.updatePolicy === 1 ? '强制更新' : '提示更新';
    renderSettingsWarnings();
  } catch (error) {
    connectionStatus.textContent = '连接异常';
  }
}

async function saveSettings() {
  const payload = {
    idleThresholdSeconds: Number(document.getElementById('idleThreshold').value || 0),
    heartbeatIntervalSeconds: Number(document.getElementById('heartbeatInterval').value || 0),
    offlineThresholdSeconds: Number(document.getElementById('offlineThreshold').value || 0),
    fishRatioWarnPercent: Number(document.getElementById('fishRatioWarn').value || 0),
    updatePolicy: Number(document.getElementById('updatePolicy').value || 0),
    latestVersion: document.getElementById('latestVersion').value.trim(),
    updateUrl: document.getElementById('updateUrl').value.trim(),
  };

  const warnings = renderSettingsWarnings();
  if (warnings.length > 0) {
    setStatus('请先修正配置提示内容', document.getElementById('settingsStatus'));
    return;
  }

  try {
    await fetchJSON('/api/v1/admin/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    updatePolicyValue.textContent = payload.updatePolicy === 1 ? '强制更新' : '提示更新';
    setStatus('保存成功', document.getElementById('settingsStatus'));
    settingsCache = payload;
  } catch (error) {
    setStatus(error.message, document.getElementById('settingsStatus'));
  }
}
async function loadRules() {
  try {
    const rules = await fetchJSON('/api/v1/admin/rules');
    rulesCache = Array.isArray(rules) ? rules : [];
    renderRules(rulesCache);
  } catch (error) {
    setStatus(error.message, document.getElementById('ruleStatus'));
  }
}

function resetRuleForm() {
  editingRuleId = null;
  document.getElementById('ruleType').value = 'white';
  document.getElementById('matchMode').value = 'process';
  document.getElementById('matchValue').value = '';
  document.getElementById('ruleRemark').value = '';
  document.getElementById('ruleEnabled').checked = true;
  document.getElementById('createRule').textContent = '保存规则';
}

function fillRuleForm(rule) {
  editingRuleId = rule.id;
  document.getElementById('ruleType').value = rule.type;
  document.getElementById('matchMode').value = rule.matchMode;
  document.getElementById('matchValue').value = rule.matchValue;
  document.getElementById('ruleRemark').value = rule.remark || '';
  document.getElementById('ruleEnabled').checked = !!rule.enabled;
  document.getElementById('createRule').textContent = '更新规则';
}

function focusRuleForm() {
  const target = document.getElementById('matchValue');
  if (!target) {
    return;
  }
  target.scrollIntoView({ behavior: 'smooth', block: 'center' });
  target.focus();
}

async function submitRule() {
  const matchValue = document.getElementById('matchValue').value.trim();
  if (!matchValue) {
    setStatus('请输入匹配值', document.getElementById('ruleStatus'));
    return;
  }
  const payload = {
    id: editingRuleId || 0,
    type: document.getElementById('ruleType').value,
    matchMode: document.getElementById('matchMode').value,
    matchValue: matchValue,
    enabled: document.getElementById('ruleEnabled').checked,
    remark: document.getElementById('ruleRemark').value.trim(),
  };

  try {
    await fetchJSON('/api/v1/admin/rules', {
      method: editingRuleId ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', document.getElementById('ruleStatus'));
    resetRuleForm();
    loadRules();
  } catch (error) {
    setStatus(error.message, document.getElementById('ruleStatus'));
  }
}

async function toggleRule(id) {
  const rule = rulesCache.find((item) => item.id === id);
  if (!rule) return;
  const payload = {
    id: rule.id,
    type: rule.type,
    matchMode: rule.matchMode,
    matchValue: rule.matchValue,
    enabled: !rule.enabled,
    remark: rule.remark || '',
  };
  try {
    await fetchJSON('/api/v1/admin/rules', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    loadRules();
  } catch (error) {
    setStatus(error.message, document.getElementById('ruleStatus'));
  }
}

async function deleteRule(id) {
  try {
    await fetchJSON('/api/v1/admin/rules?id=' + id, { method: 'DELETE' });
    loadRules();
  } catch (error) {
    setStatus(error.message, document.getElementById('ruleStatus'));
  }
}

function renderRules(rules) {
  const container = document.getElementById('rulesTable');
  if (!rules || rules.length === 0) {
    container.innerHTML = '<div class="empty-hint">暂无规则</div>';
    return;
  }
  const header = [
    '<div class="table-row table-header cols-7">',
    '<div>类型</div>',
    '<div>匹配方式</div>',
    '<div>匹配值</div>',
    '<div>状态</div>',
    '<div>备注</div>',
    '<div>创建时间</div>',
    '<div>操作</div>',
    '</div>'
  ].join('');

  const rows = rules.map((rule) => {
    const typeClass = rule.type === 'black' ? 'black' : '';
    const enabledLabel = rule.enabled ? '启用' : '停用';
    const toggleLabel = rule.enabled ? '停用' : '启用';
    return [
      '<div class="table-row cols-7">',
      '<div><span class="tag ' + typeClass + '">' + rule.typeLabel + '</span></div>',
      '<div>' + rule.matchModeLabel + '</div>',
      '<div>' + rule.matchValue + '</div>',
      '<div>' + enabledLabel + '</div>',
      '<div>' + (rule.remark || '-') + '</div>',
      '<div>' + (rule.createdAt || '-') + '</div>',
      '<div>',
      '<button class="btn btn-secondary" data-action="edit" data-id="' + rule.id + '">编辑</button>',
      '<button class="btn btn-secondary" data-action="toggle" data-id="' + rule.id + '">' + toggleLabel + '</button>',
      '<button class="btn btn-secondary" data-action="delete" data-id="' + rule.id + '">删除</button>',
      '</div>',
      '</div>'
    ].join('');
  });

  container.innerHTML = header + rows.join('');
}
function updateLiveMap(items) {
  liveMap = {};
  items.forEach((item) => {
    liveMap[item.employeeCode] = item;
  });
}

function getLiveDisplay(item) {
  const offlineThreshold = settingsCache ? settingsCache.offlineThresholdSeconds : 0;
  let statusCode = item.statusCode || 'normal';
  let statusLabel = item.statusLabel || '常规';
  let remaining = null;
  const delaySeconds = item.delaySeconds || 0;
  if (offlineThreshold > 0) {
    if (delaySeconds > offlineThreshold) {
      statusCode = 'offline';
      statusLabel = '离线';
      remaining = 0;
    } else {
      remaining = offlineThreshold - delaySeconds;
    }
  }
  return { statusCode: statusCode, statusLabel: statusLabel, remaining: remaining };
}

function renderLiveGrid() {
  const container = document.getElementById('liveGrid');
  const empty = document.getElementById('liveEmpty');
  const keyword = document.getElementById('liveSearch').value.trim().toLowerCase();
  const filtered = liveItems.filter((item) => {
    if (!keyword) return true;
    const text = [item.employeeCode, item.name, item.department || ''].join(' ').toLowerCase();
    return text.includes(keyword);
  });

  if (filtered.length === 0) {
    container.innerHTML = '';
    empty.style.display = 'block';
    return;
  }
  empty.style.display = 'none';

  const groups = {};
  filtered.forEach((item) => {
    const dept = item.department || '未分配';
    if (!groups[dept]) {
      groups[dept] = [];
    }
    groups[dept].push(item);
  });

  const groupHtml = Object.keys(groups).sort().map((dept) => {
    const cards = groups[dept].map((item) => {
      const display = getLiveDisplay(item);
      const description = item.description || '无活动';
      return [
        '<div class="live-card" data-code="' + item.employeeCode + '">',
        '<div class="status ' + display.statusCode + '">' + display.statusLabel + '</div>',
        '<div class="name">' + item.name + '（' + item.employeeCode + '）</div>',
        '<div class="desc">' + description + '</div>',
        '<div class="meta">' + buildMetaText(item.lastSeen, display.remaining) + '</div>',
        '</div>'
      ].join('');
    }).join('');
    return [
      '<div class="live-group">',
      '<div class="live-group-title">' + dept + '</div>',
      '<div class="live-grid">' + cards + '</div>',
      '</div>'
    ].join('');
  });

  container.innerHTML = groupHtml.join('');
}

function updateLiveCard(card, item) {
  const display = getLiveDisplay(item);
  const statusEl = card.querySelector('.status');
  const metaEl = card.querySelector('.meta');
  if (statusEl) {
    statusEl.textContent = display.statusLabel;
    statusEl.className = 'status ' + display.statusCode;
  }
  if (metaEl) {
    metaEl.textContent = buildMetaText(item.lastSeen, display.remaining);
  }
}

function startLiveTimer() {
  if (liveTimer) {
    clearInterval(liveTimer);
  }
  liveTimer = setInterval(() => {
    liveItems.forEach((item) => {
      item.delaySeconds = (item.delaySeconds || 0) + 1;
    });
    document.querySelectorAll('.live-card').forEach((card) => {
      const code = card.dataset.code;
      const item = liveMap[code];
      if (item) {
        updateLiveCard(card, item);
      }
    });
  }, 1000);
}

async function loadLiveSnapshot() {
  try {
    const data = await fetchJSON('/api/v1/admin/live-snapshot');
    liveItems = Array.isArray(data) ? data : [];
    updateLiveMap(liveItems);
    renderLiveGrid();
    startLiveTimer();
  } catch (error) {
    connectionStatus.textContent = '连接异常';
  }
}

function applyLiveUpdate(item) {
  if (!item || !item.employeeCode) return;
  const existing = liveMap[item.employeeCode];
  if (existing) {
    Object.assign(existing, item);
  } else {
    liveItems.push(item);
  }
  updateLiveMap(liveItems);
  renderLiveGrid();
}

function connectLiveWS() {
  if (!authToken) return;
  if (wsClient && (wsClient.readyState === WebSocket.OPEN || wsClient.readyState === WebSocket.CONNECTING)) {
    return;
  }
  const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
  const wsUrl = scheme + '://' + location.host + '/ws/v1/live?token=' + encodeURIComponent(authToken);
  wsClient = new WebSocket(wsUrl);
  wsClient.onopen = () => {
    connectionStatus.textContent = '已连接';
  };
  wsClient.onclose = () => {
    connectionStatus.textContent = '连接中断';
    wsClient = null;
    if (authToken) {
      setTimeout(connectLiveWS, 3000);
    }
  };
  wsClient.onerror = () => {
    connectionStatus.textContent = '连接异常';
  };
  wsClient.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data || '{}');
      if (data.type === 'snapshot' && Array.isArray(data.items)) {
        liveItems = data.items;
        updateLiveMap(liveItems);
        renderLiveGrid();
        startLiveTimer();
        return;
      }
      if (data.type === 'update' && data.item) {
        applyLiveUpdate(data.item);
      }
    } catch (error) {
      return;
    }
  };
}
function renderTable(container, headers, rows, colsClass) {
  if (!container) return;
  if (!rows || rows.length === 0) {
    container.innerHTML = '<div class="empty-hint">暂无数据</div>';
    return;
  }
  const headerHtml = '<div class="table-row table-header ' + colsClass + '">' + headers.map((item) => '<div>' + item + '</div>').join('') + '</div>';
  container.innerHTML = headerHtml + rows.join('');
}

async function loadDailyReport() {
  const dateInput = document.getElementById('reportDate');
  const date = dateInput.value || new Date().toISOString().slice(0, 10);
  const deptId = document.getElementById('reportDepartment').value;
  let url = '/api/v1/admin/reports/daily?date=' + encodeURIComponent(date);
  if (deptId && Number(deptId) > 0) {
    url += '&departmentId=' + deptId;
  }
  try {
    const data = await fetchJSON(url);
    renderDailyReport(data.items || []);
  } catch (error) {
    document.getElementById('dailyReportTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderDailyReport(items) {
  const container = document.getElementById('dailyReportTable');
  const headers = ['工号', '姓名', '部门', '工作', '常规', '摸鱼', '离开', '离线', '在岗', '有效'];
  const rows = items.map((item) => [
    '<div class="table-row cols-10">',
    '<div>' + item.employeeCode + '</div>',
    '<div>' + item.name + '</div>',
    '<div>' + (item.department || '-') + '</div>',
    '<div>' + item.workDuration + '</div>',
    '<div>' + item.normalDuration + '</div>',
    '<div>' + item.fishDuration + '</div>',
    '<div>' + item.idleDuration + '</div>',
    '<div>' + item.offlineDuration + '</div>',
    '<div>' + item.attendanceDuration + '</div>',
    '<div>' + item.effectiveDuration + '</div>',
    '</div>'
  ].join(''));
  renderTable(container, headers, rows, 'cols-10');
}

async function exportDaily() {
  const dateInput = document.getElementById('reportDate');
  const date = dateInput.value || new Date().toISOString().slice(0, 10);
  const deptId = document.getElementById('reportDepartment').value;
  let url = '/api/v1/admin/exports/daily.xlsx?date=' + encodeURIComponent(date);
  if (deptId && Number(deptId) > 0) {
    url += '&departmentId=' + deptId;
  }
  try {
    const blob = await fetchBlob(url);
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = '日报_' + date + '.xlsx';
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(link.href);
  } catch (error) {
    document.getElementById('dailyReportTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

async function loadTimeline() {
  const code = document.getElementById('timelineEmployee').value;
  const dateInput = document.getElementById('timelineDate');
  const date = dateInput.value || new Date().toISOString().slice(0, 10);
  if (!code) {
    document.getElementById('timelineTable').innerHTML = '<div class="empty-hint">请选择员工</div>';
    return;
  }
  try {
    const data = await fetchJSON('/api/v1/admin/reports/timeline?employeeCode=' + encodeURIComponent(code) + '&date=' + encodeURIComponent(date));
    renderTimeline(data.items || []);
  } catch (error) {
    document.getElementById('timelineTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderTimeline(items) {
  const container = document.getElementById('timelineTable');
  const headers = ['状态', '开始', '结束', '时长', '描述', '来源'];
  const rows = items.map((item) => [
    '<div class="table-row cols-6">',
    '<div>' + item.statusLabel + '</div>',
    '<div>' + item.startAt + '</div>',
    '<div>' + item.endAt + '</div>',
    '<div>' + item.duration + '</div>',
    '<div>' + (item.description || '-') + '</div>',
    '<div>' + item.sourceLabel + '</div>',
    '</div>'
  ].join(''));
  renderTable(container, headers, rows, 'cols-6');
}

async function loadRank() {
  const dateInput = document.getElementById('reportDate');
  const date = dateInput.value || new Date().toISOString().slice(0, 10);
  try {
    const data = await fetchJSON('/api/v1/admin/reports/rank?date=' + encodeURIComponent(date));
    renderRankTables(data);
  } catch (error) {
    document.getElementById('rankWorkTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
    document.getElementById('rankFishTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderRankTables(data) {
  const workHeaders = ['排名', '工号', '姓名', '部门', '有效工时'];
  const fishHeaders = ['排名', '工号', '姓名', '部门', '摸鱼比例'];
  const workRows = (data.workTop || []).map((item, index) => [
    '<div class="table-row cols-5">',
    '<div>' + (index + 1) + '</div>',
    '<div>' + item.employeeCode + '</div>',
    '<div>' + item.name + '</div>',
    '<div>' + (item.department || '-') + '</div>',
    '<div>' + item.value + '</div>',
    '</div>'
  ].join(''));
  const fishRows = (data.fishTop || []).map((item, index) => [
    '<div class="table-row cols-5">',
    '<div>' + (index + 1) + '</div>',
    '<div>' + item.employeeCode + '</div>',
    '<div>' + item.name + '</div>',
    '<div>' + (item.department || '-') + '</div>',
    '<div>' + item.value + '</div>',
    '</div>'
  ].join(''));
  renderTable(document.getElementById('rankWorkTable'), workHeaders, workRows, 'cols-5');
  renderTable(document.getElementById('rankFishTable'), fishHeaders, fishRows, 'cols-5');
}
function getDateFromInput(value) {
  if (!value) {
    return new Date().toISOString().slice(0, 10);
  }
  return value.slice(0, 10);
}

async function loadAdjustments() {
  const date = getDateFromInput(document.getElementById('adjustStart').value);
  try {
    const items = await fetchJSON('/api/v1/admin/manual-adjustments?date=' + encodeURIComponent(date));
    renderAdjustments(items || []);
  } catch (error) {
    document.getElementById('adjustmentsTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function resetAdjustmentForm() {
  editingAdjustmentId = null;
  document.getElementById('adjustEmployee').value = '';
  document.getElementById('adjustStart').value = '';
  document.getElementById('adjustEnd').value = '';
  document.getElementById('adjustReason').value = '停电';
  document.getElementById('adjustNote').value = '';
  document.getElementById('createAdjustment').textContent = '提交补录';
}

function fillAdjustmentForm(item) {
  editingAdjustmentId = item.id;
  document.getElementById('adjustEmployee').value = item.employeeCode;
  document.getElementById('adjustStart').value = formatDateTimeLocal(item.startAt);
  document.getElementById('adjustEnd').value = formatDateTimeLocal(item.endAt);
  document.getElementById('adjustReason').value = item.reason;
  document.getElementById('adjustNote').value = item.note;
  document.getElementById('createAdjustment').textContent = '更新补录';
}

async function submitAdjustment() {
  const payload = {
    id: editingAdjustmentId || 0,
    employeeCode: document.getElementById('adjustEmployee').value,
    startAt: document.getElementById('adjustStart').value,
    endAt: document.getElementById('adjustEnd').value,
    reason: document.getElementById('adjustReason').value,
    note: document.getElementById('adjustNote').value.trim(),
  };
  if (!payload.employeeCode || !payload.startAt || !payload.endAt || !payload.reason || !payload.note) {
    setStatus('请完整填写补录信息', document.getElementById('adjustStatus'));
    return;
  }
  try {
    await fetchJSON('/api/v1/admin/manual-adjustments', {
      method: editingAdjustmentId ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', document.getElementById('adjustStatus'));
    resetAdjustmentForm();
    loadAdjustments();
  } catch (error) {
    setStatus(error.message, document.getElementById('adjustStatus'));
  }
}

async function revokeAdjustment(id) {
  try {
    await fetchJSON('/api/v1/admin/manual-adjustments?id=' + id, { method: 'DELETE' });
    loadAdjustments();
  } catch (error) {
    setStatus(error.message, document.getElementById('adjustStatus'));
  }
}

function renderAdjustments(items) {
  adjustmentsCache = items;
  const container = document.getElementById('adjustmentsTable');
  const headers = ['工号', '姓名', '部门', '开始', '结束', '原因', '备注', '状态', '创建时间', '操作'];
  const rows = items.map((item) => {
    const actions = item.status === 'active'
      ? '<button class="btn btn-secondary" data-action="edit" data-id="' + item.id + '">编辑</button>' +
        '<button class="btn btn-secondary" data-action="revoke" data-id="' + item.id + '">撤销</button>'
      : '<span class="muted">已撤销</span>';
    return [
      '<div class="table-row cols-10">',
      '<div>' + item.employeeCode + '</div>',
      '<div>' + item.name + '</div>',
      '<div>' + (item.department || '-') + '</div>',
      '<div>' + item.startAt + '</div>',
      '<div>' + item.endAt + '</div>',
      '<div>' + item.reason + '</div>',
      '<div>' + item.note + '</div>',
      '<div>' + item.statusLabel + '</div>',
      '<div>' + item.createdAt + '</div>',
      '<div>' + actions + '</div>',
      '</div>'
    ].join('');
  });
  renderTable(container, headers, rows, 'cols-10');
}

async function loadIncidents() {
  const date = getDateFromInput(document.getElementById('incidentStart').value);
  try {
    const items = await fetchJSON('/api/v1/admin/system-incidents?date=' + encodeURIComponent(date));
    renderIncidents(items || []);
  } catch (error) {
    document.getElementById('incidentsTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function resetIncidentForm() {
  editingIncidentId = null;
  document.getElementById('incidentStart').value = '';
  document.getElementById('incidentEnd').value = '';
  document.getElementById('incidentReason').value = '';
  document.getElementById('incidentNote').value = '';
  document.getElementById('createIncident').textContent = '登记事故';
}

function fillIncidentForm(item) {
  editingIncidentId = item.id;
  document.getElementById('incidentStart').value = formatDateTimeLocal(item.startAt);
  document.getElementById('incidentEnd').value = formatDateTimeLocal(item.endAt);
  document.getElementById('incidentReason').value = item.reason;
  document.getElementById('incidentNote').value = item.note || '';
  document.getElementById('createIncident').textContent = '更新事故';
}

async function submitIncident() {
  const payload = {
    id: editingIncidentId || 0,
    startAt: document.getElementById('incidentStart').value,
    endAt: document.getElementById('incidentEnd').value,
    reason: document.getElementById('incidentReason').value.trim(),
    note: document.getElementById('incidentNote').value.trim(),
  };
  if (!payload.startAt || !payload.endAt || !payload.reason) {
    setStatus('请填写事故时间和原因', document.getElementById('incidentStatus'));
    return;
  }
  try {
    await fetchJSON('/api/v1/admin/system-incidents', {
      method: editingIncidentId ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', document.getElementById('incidentStatus'));
    resetIncidentForm();
    loadIncidents();
  } catch (error) {
    setStatus(error.message, document.getElementById('incidentStatus'));
  }
}

async function deleteIncident(id) {
  try {
    await fetchJSON('/api/v1/admin/system-incidents?id=' + id, { method: 'DELETE' });
    loadIncidents();
  } catch (error) {
    setStatus(error.message, document.getElementById('incidentStatus'));
  }
}

function renderIncidents(items) {
  incidentsCache = items;
  const container = document.getElementById('incidentsTable');
  const headers = ['开始', '结束', '原因', '备注', '创建时间', '操作'];
  const rows = items.map((item) => [
    '<div class="table-row cols-6">',
    '<div>' + item.startAt + '</div>',
    '<div>' + item.endAt + '</div>',
    '<div>' + item.reason + '</div>',
    '<div>' + (item.note || '-') + '</div>',
    '<div>' + item.createdAt + '</div>',
    '<div>',
    '<button class="btn btn-secondary" data-action="edit" data-id="' + item.id + '">编辑</button>',
    '<button class="btn btn-secondary" data-action="delete" data-id="' + item.id + '">删除</button>',
    '</div>',
    '</div>'
  ].join(''));
  renderTable(container, headers, rows, 'cols-6');
}

async function loadOfflineSegments() {
  const dateInput = document.getElementById('offlineDate');
  const date = dateInput.value || new Date().toISOString().slice(0, 10);
  const code = document.getElementById('offlineEmployee').value;
  let url = '/api/v1/admin/offline-segments?date=' + encodeURIComponent(date);
  if (code) {
    url += '&employeeCode=' + encodeURIComponent(code);
  }
  try {
    const items = await fetchJSON(url);
    renderOfflineSegments(items || []);
  } catch (error) {
    document.getElementById('offlineSegmentsTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderOfflineSegments(items) {
  const container = document.getElementById('offlineSegmentsTable');
  const headers = ['工号', '姓名', '部门', '开始', '结束', '时长'];
  const rows = items.map((item) => [
    '<div class="table-row cols-6">',
    '<div>' + item.employeeCode + '</div>',
    '<div>' + item.name + '</div>',
    '<div>' + (item.department || '-') + '</div>',
    '<div>' + item.startAt + '</div>',
    '<div>' + item.endAt + '</div>',
    '<div>' + item.duration + '</div>',
    '</div>'
  ].join(''));
  renderTable(container, headers, rows, 'cols-6');
}

async function loadAuditLogs() {
  const dateInput = document.getElementById('auditDate');
  const date = dateInput.value || new Date().toISOString().slice(0, 10);
  try {
    const items = await fetchJSON('/api/v1/admin/audit-logs?date=' + encodeURIComponent(date));
    renderAuditLogs(items || []);
  } catch (error) {
    document.getElementById('auditTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderAuditLogs(items) {
  const container = document.getElementById('auditTable');
  const headers = ['操作', '目标', '操作人', '详情', '时间'];
  const rows = items.map((item) => [
    '<div class="table-row cols-5">',
    '<div>' + item.action + '</div>',
    '<div>' + item.target + '</div>',
    '<div>' + item.operator + '</div>',
    '<div>' + (item.detail || '-') + '</div>',
    '<div>' + item.createdAt + '</div>',
    '</div>'
  ].join(''));
  renderTable(container, headers, rows, 'cols-5');
}
async function loadDepartments() {
  try {
    const items = await fetchJSON('/api/v1/admin/departments');
    departments = Array.isArray(items) ? items : [];
    renderDepartmentOptions();
    renderDepartmentsTable(departments);
  } catch (error) {
    document.getElementById('departmentsTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderDepartmentOptions() {
  const parentSelect = document.getElementById('deptParent');
  const reportSelect = document.getElementById('reportDepartment');
  const employeeSelect = document.getElementById('employeeDepartment');
  const options = departments.map((dept) => '<option value="' + dept.id + '">' + dept.name + '</option>').join('');
  if (parentSelect) {
    parentSelect.innerHTML = '<option value="0">无</option>' + options;
  }
  if (reportSelect) {
    reportSelect.innerHTML = '<option value="0">全部</option>' + options;
  }
  if (employeeSelect) {
    employeeSelect.innerHTML = '<option value="0">未分配</option>' + options;
  }
}

function resetDepartmentForm() {
  editingDepartmentId = null;
  document.getElementById('deptName').value = '';
  document.getElementById('deptParent').value = '0';
  document.getElementById('saveDepartment').textContent = '保存部门';
}

function fillDepartmentForm(item) {
  editingDepartmentId = item.id;
  document.getElementById('deptName').value = item.name;
  document.getElementById('deptParent').value = String(item.parentId || 0);
  document.getElementById('saveDepartment').textContent = '更新部门';
}

async function saveDepartment() {
  const payload = {
    id: editingDepartmentId || 0,
    name: document.getElementById('deptName').value.trim(),
    parentId: Number(document.getElementById('deptParent').value || 0),
  };
  if (!payload.name) {
    setStatus('请输入部门名称', document.getElementById('deptStatus'));
    return;
  }
  try {
    await fetchJSON('/api/v1/admin/departments', {
      method: editingDepartmentId ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', document.getElementById('deptStatus'));
    resetDepartmentForm();
    loadDepartments();
  } catch (error) {
    setStatus(error.message, document.getElementById('deptStatus'));
  }
}

async function deleteDepartment(id) {
  try {
    await fetchJSON('/api/v1/admin/departments?id=' + id, { method: 'DELETE' });
    loadDepartments();
  } catch (error) {
    setStatus(error.message, document.getElementById('deptStatus'));
  }
}

function renderDepartmentsTable(items) {
  const container = document.getElementById('departmentsTable');
  const headers = ['部门名称', '上级部门', '操作'];
  const rows = items.map((item) => [
    '<div class="table-row cols-4">',
    '<div>' + item.name + '</div>',
    '<div>' + (item.parentName || '-') + '</div>',
    '<div>',
    '<button class="btn btn-secondary" data-action="edit" data-id="' + item.id + '">编辑</button>',
    '<button class="btn btn-secondary" data-action="delete" data-id="' + item.id + '">删除</button>',
    '</div>',
    '<div></div>',
    '</div>'
  ].join(''));
  renderTable(container, headers, rows, 'cols-4');
}

async function loadEmployees() {
  try {
    const items = await fetchJSON('/api/v1/admin/employees');
    employees = Array.isArray(items) ? items : [];
    renderEmployeeOptions();
    renderEmployeesTable(employees);
  } catch (error) {
    document.getElementById('employeesTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderEmployeeOptions() {
  const adjustSelect = document.getElementById('adjustEmployee');
  const timelineSelect = document.getElementById('timelineEmployee');
  const offlineSelect = document.getElementById('offlineEmployee');
  const options = employees.map((emp) => {
    const label = emp.name + '（' + emp.employeeCode + '）' + (emp.enabled ? '' : '（停用）');
    return '<option value="' + emp.employeeCode + '">' + label + '</option>';
  }).join('');
  if (adjustSelect) {
    adjustSelect.innerHTML = '<option value="">请选择员工</option>' + options;
  }
  if (timelineSelect) {
    timelineSelect.innerHTML = '<option value="">请选择员工</option>' + options;
  }
  if (offlineSelect) {
    offlineSelect.innerHTML = '<option value="">全部</option>' + options;
  }
}

function resetEmployeeForm() {
  editingEmployeeId = null;
  document.getElementById('employeeCode').value = '';
  document.getElementById('employeeName').value = '';
  document.getElementById('employeeDepartment').value = '0';
  document.getElementById('employeeEnabled').checked = true;
  document.getElementById('saveEmployee').textContent = '保存员工';
}

function fillEmployeeForm(item) {
  editingEmployeeId = item.id;
  document.getElementById('employeeCode').value = item.employeeCode;
  document.getElementById('employeeName').value = item.name;
  document.getElementById('employeeDepartment').value = String(item.departmentId || 0);
  document.getElementById('employeeEnabled').checked = !!item.enabled;
  document.getElementById('saveEmployee').textContent = '更新员工';
}

async function saveEmployee() {
  const payload = {
    id: editingEmployeeId || 0,
    employeeCode: document.getElementById('employeeCode').value.trim(),
    name: document.getElementById('employeeName').value.trim(),
    departmentId: Number(document.getElementById('employeeDepartment').value || 0),
    enabled: document.getElementById('employeeEnabled').checked,
  };
  if (!payload.employeeCode || !payload.name) {
    setStatus('工号与姓名不能为空', document.getElementById('employeeStatus'));
    return;
  }
  try {
    await fetchJSON('/api/v1/admin/employees', {
      method: editingEmployeeId ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', document.getElementById('employeeStatus'));
    resetEmployeeForm();
    loadEmployees();
  } catch (error) {
    setStatus(error.message, document.getElementById('employeeStatus'));
  }
}

async function unbindEmployee(id) {
  try {
    await fetchJSON('/api/v1/admin/employees/unbind', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: id }),
    });
    loadEmployees();
  } catch (error) {
    setStatus(error.message, document.getElementById('employeeStatus'));
  }
}

async function disableEmployee(id) {
  try {
    await fetchJSON('/api/v1/admin/employees?id=' + id, { method: 'DELETE' });
    loadEmployees();
  } catch (error) {
    setStatus(error.message, document.getElementById('employeeStatus'));
  }
}

function renderEmployeesTable(items) {
  const container = document.getElementById('employeesTable');
  const headers = ['工号', '姓名', '部门', '绑定状态', '最近上报', '状态', '操作'];
  const rows = items.map((item) => {
    const statusLabel = item.enabled ? '启用' : '停用';
    const actions = [
      '<button class="btn btn-secondary" data-action="edit" data-id="' + item.id + '">编辑</button>'
    ];
    if (item.bindStatus === '已绑定') {
      actions.push('<button class="btn btn-secondary" data-action="unbind" data-id="' + item.id + '">解绑</button>');
    }
    if (item.enabled) {
      actions.push('<button class="btn btn-secondary" data-action="disable" data-id="' + item.id + '">停用</button>');
    }
    return [
      '<div class="table-row cols-7">',
      '<div>' + item.employeeCode + '</div>',
      '<div>' + item.name + '</div>',
      '<div>' + (item.department || '-') + '</div>',
      '<div>' + item.bindStatus + '</div>',
      '<div>' + (item.lastSeen || '-') + '</div>',
      '<div>' + statusLabel + '</div>',
      '<div>' + actions.join('') + '</div>',
      '</div>'
    ].join('');
  });
  renderTable(container, headers, rows, 'cols-7');
}

async function initApp() {
  if (!authToken) {
    showLogin();
    return;
  }
  adminNameValue.textContent = adminName || '管理员';
  timezoneValue.textContent = (Intl.DateTimeFormat().resolvedOptions().timeZone || 'Local');
  setDefaultDates();
  await loadSettings();
  await loadDepartments();
  await loadEmployees();
  await loadRules();
  await loadLiveSnapshot();
  connectLiveWS();
  startLiveTimer();
  loadAdjustments();
  loadIncidents();
  loadOfflineSegments();
  loadAuditLogs();
}

document.getElementById('loginBtn').addEventListener('click', login);
document.getElementById('logoutBtn').addEventListener('click', logout);
document.getElementById('loginPassword').addEventListener('keydown', (event) => {
  if (event.key === 'Enter') {
    login();
  }
});

document.getElementById('saveSettings').addEventListener('click', saveSettings);
['idleThreshold', 'heartbeatInterval', 'offlineThreshold', 'fishRatioWarn'].forEach((id) => {
  const input = document.getElementById(id);
  if (input) {
    input.addEventListener('input', renderSettingsWarnings);
    input.addEventListener('change', renderSettingsWarnings);
  }
});
document.getElementById('refreshRules').addEventListener('click', loadRules);
document.getElementById('createRule').addEventListener('click', submitRule);
document.getElementById('cancelRule').addEventListener('click', resetRuleForm);

document.getElementById('refreshLive').addEventListener('click', loadLiveSnapshot);
document.getElementById('liveSearch').addEventListener('input', renderLiveGrid);

document.getElementById('loadDailyReport').addEventListener('click', loadDailyReport);
document.getElementById('exportDaily').addEventListener('click', exportDaily);
document.getElementById('loadTimeline').addEventListener('click', loadTimeline);
document.getElementById('loadRank').addEventListener('click', loadRank);

document.getElementById('createAdjustment').addEventListener('click', submitAdjustment);
document.getElementById('cancelAdjustment').addEventListener('click', resetAdjustmentForm);

document.getElementById('createIncident').addEventListener('click', submitIncident);
document.getElementById('cancelIncident').addEventListener('click', resetIncidentForm);

document.getElementById('loadOfflineSegments').addEventListener('click', loadOfflineSegments);
document.getElementById('loadAudit').addEventListener('click', loadAuditLogs);

document.getElementById('saveDepartment').addEventListener('click', saveDepartment);
document.getElementById('cancelDepartment').addEventListener('click', resetDepartmentForm);

document.getElementById('saveEmployee').addEventListener('click', saveEmployee);
document.getElementById('cancelEmployee').addEventListener('click', resetEmployeeForm);

document.getElementById('rulesTable').addEventListener('click', (event) => {
  const btn = event.target.closest('button');
  if (!btn) return;
  const action = btn.dataset.action;
  const id = Number(btn.dataset.id || 0);
  if (!id) return;
  if (action === 'edit') {
    const rule = rulesCache.find((item) => item.id === id);
    if (rule) {
      fillRuleForm(rule);
      focusRuleForm();
    }
  }
  if (action === 'toggle') {
    toggleRule(id);
  }
  if (action === 'delete') {
    deleteRule(id);
  }
});

document.getElementById('adjustmentsTable').addEventListener('click', (event) => {
  const btn = event.target.closest('button');
  if (!btn) return;
  const action = btn.dataset.action;
  const id = Number(btn.dataset.id || 0);
  if (!id) return;
  if (action === 'edit') {
    const adjustment = adjustmentsCache.find((row) => row.id === id);
    if (adjustment) {
      fillAdjustmentForm(adjustment);
    }
  }
  if (action === 'revoke') {
    revokeAdjustment(id);
  }
});

document.getElementById('incidentsTable').addEventListener('click', (event) => {
  const btn = event.target.closest('button');
  if (!btn) return;
  const action = btn.dataset.action;
  const id = Number(btn.dataset.id || 0);
  if (!id) return;
  if (action === 'edit') {
    const incident = incidentsCache.find((row) => row.id === id);
    if (incident) {
      fillIncidentForm(incident);
    }
  }
  if (action === 'delete') {
    deleteIncident(id);
  }
});

document.getElementById('departmentsTable').addEventListener('click', (event) => {
  const btn = event.target.closest('button');
  if (!btn) return;
  const action = btn.dataset.action;
  const id = Number(btn.dataset.id || 0);
  if (!id) return;
  if (action === 'edit') {
    const item = departments.find((row) => row.id === id);
    if (item) {
      fillDepartmentForm(item);
    }
  }
  if (action === 'delete') {
    deleteDepartment(id);
  }
});

document.getElementById('employeesTable').addEventListener('click', (event) => {
  const btn = event.target.closest('button');
  if (!btn) return;
  const action = btn.dataset.action;
  const id = Number(btn.dataset.id || 0);
  if (!id) return;
  if (action === 'edit') {
    const item = employees.find((row) => row.id === id);
    if (item) {
      fillEmployeeForm(item);
    }
  }
  if (action === 'unbind') {
    unbindEmployee(id);
  }
  if (action === 'disable') {
    disableEmployee(id);
  }
});

switchSection('live');

if (authToken) {
  setAuth(authToken, adminName);
  initApp();
} else {
  showLogin();
}




