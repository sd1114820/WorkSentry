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
let timelineSearchReady = false;

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

let attendanceRules = [];
let attendanceThresholds = [];
let editingAttendanceThresholdIndex = null;
let checkoutTemplates = [];
let checkoutFields = [];
let checkoutRecordsCache = [];
let checkoutRecordsTotal = 0;
let checkoutRecordsPage = 1;
let checkoutReviewCache = [];
let checkoutReviewTotal = 0;
let reviewPage = 1;
let editingCheckoutTemplateId = null;
let editingCheckoutFieldId = null;
let activeCheckoutTemplateId = null;
const employeeSearchReady = {};


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
  ['reportDate', 'timelineDate', 'offlineDate', 'auditDate', 'checkoutQueryStart', 'checkoutQueryEnd', 'reviewStartDate', 'reviewEndDate'].forEach((id) => {
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
  if (item && (item.working === false || statusCode === 'offwork')) {
    statusCode = 'offwork';
    statusLabel = item.statusLabel || '已下班';
    return { statusCode: statusCode, statusLabel: statusLabel, remaining: null };
  }
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
  const hidden = document.getElementById('timelineEmployee').value;
  const typed = document.getElementById('timelineEmployeeSearch').value;
  const code = resolveEmployeeCode(hidden, typed);
  const dateInput = document.getElementById('timelineDate');
  const date = dateInput.value || new Date().toISOString().slice(0, 10);
  if (!code) {
    updateTimelineHint('请选择员工');
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

function parseClockDurationSeconds(value) {
  const text = String(value || '').trim();
  if (!text) return 0;
  const parts = text.split(':').map((item) => Number(item));
  if (!parts.length || parts.some((item) => Number.isNaN(item))) {
    return 0;
  }
  if (parts.length === 2) {
    return parts[0] * 3600 + parts[1] * 60;
  }
  if (parts.length === 3) {
    return parts[0] * 3600 + parts[1] * 60 + parts[2];
  }
  return 0;
}

function formatClockDuration(seconds) {
  const total = Math.max(0, Number(seconds) || 0);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  return String(hours).padStart(2, '0') + ':' + String(minutes).padStart(2, '0');
}

function mergeTimelineForDisplay(items) {
  const list = Array.isArray(items) ? items : [];
  const groups = [];

  for (const item of list) {
    const durationSeconds = parseClockDurationSeconds(item.duration);
    const last = groups[groups.length - 1];
    if (last && last.statusCode === item.statusCode && last.endAt === item.startAt) {
      last.endAt = item.endAt;
      last.durationSeconds += durationSeconds;
      last.children.push(item);
      if (item.description) {
        last.descriptionSet.add(item.description);
      }
      if (item.sourceLabel) {
        last.sourceSet.add(item.sourceLabel);
      }
      continue;
    }

    const group = {
      statusCode: item.statusCode,
      statusLabel: item.statusLabel,
      startAt: item.startAt,
      endAt: item.endAt,
      durationSeconds: durationSeconds,
      children: [item],
      descriptionSet: new Set(),
      sourceSet: new Set(),
    };
    if (item.description) {
      group.descriptionSet.add(item.description);
    }
    if (item.sourceLabel) {
      group.sourceSet.add(item.sourceLabel);
    }
    groups.push(group);
  }

  return groups.map((group) => {
    const descriptions = Array.from(group.descriptionSet).filter((item) => String(item || '').trim());
    const sources = Array.from(group.sourceSet).filter((item) => String(item || '').trim());

    const descriptionText = descriptions.length === 0
      ? '-'
      : (descriptions.length === 1 ? descriptions[0] : '多条记录（展开查看）');

    const sourceText = sources.length === 0
      ? '-'
      : (sources.length === 1 ? sources[0] : '混合');

    return {
      statusCode: group.statusCode,
      statusLabel: group.statusLabel,
      startAt: group.startAt,
      endAt: group.endAt,
      duration: formatClockDuration(group.durationSeconds),
      description: descriptionText,
      sourceLabel: sourceText,
      children: group.children,
    };
  });
}

function renderTimeline(items) {
  const container = document.getElementById('timelineTable');
  const headers = ['状态', '开始', '结束', '时长', '描述', '来源'];
  const groups = mergeTimelineForDisplay(items);

  const rows = [];
  groups.forEach((group, index) => {
    const canExpand = group.children && group.children.length > 1;
    const toggle = canExpand
      ? '<button class="btn btn-secondary timeline-toggle" data-group="' + index + '">展开</button>'
      : '';
    const tag = canExpand
      ? '<span class="tag">' + group.children.length + ' 段</span>'
      : '';

    rows.push([
      '<div class="table-row cols-6 timeline-group">',
      '<div>' + toggle + '<span>' + group.statusLabel + '</span> ' + tag + '</div>',
      '<div>' + group.startAt + '</div>',
      '<div>' + group.endAt + '</div>',
      '<div>' + group.duration + '</div>',
      '<div>' + (group.description || '-') + '</div>',
      '<div>' + (group.sourceLabel || '-') + '</div>',
      '</div>'
    ].join(''));

    if (!canExpand) {
      return;
    }

    group.children.forEach((item) => {
      rows.push([
        '<div class="table-row cols-6 timeline-child" data-parent="' + index + '" style="display:none;">',
        '<div>' + item.statusLabel + '</div>',
        '<div>' + item.startAt + '</div>',
        '<div>' + item.endAt + '</div>',
        '<div>' + item.duration + '</div>',
        '<div>' + (item.description || '-') + '</div>',
        '<div>' + item.sourceLabel + '</div>',
        '</div>'
      ].join(''));
    });
  });

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
  setEmployeeSearchValue('adjustEmployee', 'adjustEmployeeSearch', '');
  document.getElementById('adjustStart').value = '';
  document.getElementById('adjustEnd').value = '';
  document.getElementById('adjustReason').value = '停电';
  document.getElementById('adjustNote').value = '';
  document.getElementById('createAdjustment').textContent = '提交补录';
}

function fillAdjustmentForm(item) {
  editingAdjustmentId = item.id;
  setEmployeeSearchValue('adjustEmployee', 'adjustEmployeeSearch', item.employeeCode);
  document.getElementById('adjustStart').value = formatDateTimeLocal(item.startAt);
  document.getElementById('adjustEnd').value = formatDateTimeLocal(item.endAt);
  document.getElementById('adjustReason').value = item.reason;
  document.getElementById('adjustNote').value = item.note;
  document.getElementById('createAdjustment').textContent = '更新补录';
}

async function submitAdjustment() {
  const hidden = document.getElementById('adjustEmployee').value;
  const typed = document.getElementById('adjustEmployeeSearch').value;
  const code = resolveEmployeeCode(hidden, typed);
  const payload = {
    id: editingAdjustmentId || 0,
    employeeCode: code,
    startAt: document.getElementById('adjustStart').value,
    endAt: document.getElementById('adjustEnd').value,
    reason: document.getElementById('adjustReason').value,
    note: document.getElementById('adjustNote').value.trim(),
  };
  if (!payload.employeeCode) {
    setStatus('请选择员工', document.getElementById('adjustStatus'));
    return;
  }
  if (!payload.startAt || !payload.endAt || !payload.reason || !payload.note) {
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
  const hidden = document.getElementById('offlineEmployee').value;
  const typed = document.getElementById('offlineEmployeeSearch').value;
  const code = resolveEmployeeCode(hidden, typed);
  if (!code && typed.trim()) {
    document.getElementById('offlineSegmentsTable').innerHTML = '<div class="empty-hint">请选择员工</div>';
    return;
  }
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

function mergeOfflineSegmentsForDisplay(items) {
  const list = Array.isArray(items) ? items : [];
  const merged = [];

  for (const item of list) {
    const durationSeconds = parseClockDurationSeconds(item.duration);
    const last = merged[merged.length - 1];
    if (last && last.employeeCode === item.employeeCode && last.endAt === item.startAt) {
      last.endAt = item.endAt;
      last.durationSeconds += durationSeconds;
      continue;
    }

    merged.push({
      employeeCode: item.employeeCode,
      name: item.name,
      department: item.department,
      startAt: item.startAt,
      endAt: item.endAt,
      durationSeconds: durationSeconds,
    });
  }

  return merged.map((item) => ({
    employeeCode: item.employeeCode,
    name: item.name,
    department: item.department,
    startAt: item.startAt,
    endAt: item.endAt,
    duration: formatClockDuration(item.durationSeconds),
  }));
}

function renderOfflineSegments(items) {
  const container = document.getElementById('offlineSegmentsTable');
  const headers = ['工号', '姓名', '部门', '开始', '结束', '时长'];
  const mergedItems = mergeOfflineSegmentsForDisplay(items);
  const rows = mergedItems.map((item) => [
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
  const attendanceSelect = document.getElementById('attendanceDepartment');
  const reviewSelect = document.getElementById('reviewDepartment');
  const checkoutSelect = document.getElementById('checkoutDepartment');
  const checkoutQuerySelect = document.getElementById('checkoutQueryDepartment');
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
  if (attendanceSelect) {
    attendanceSelect.innerHTML = '<option value="0">请选择部门</option>' + options;
  }
  if (reviewSelect) {
    reviewSelect.innerHTML = '<option value="0">全部</option>' + options;
  }
  if (checkoutSelect) {
    checkoutSelect.innerHTML = '<option value="0">请选择部门</option>' + options;
  }
  if (checkoutQuerySelect) {
    checkoutQuerySelect.innerHTML = '<option value="0">全部</option>' + options;
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
    initEmployeeSearches();
  } catch (error) {
    document.getElementById('employeesTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
    updateTimelineHint(error.message || '员工列表加载失败');
  }
}

function formatEmployeeLabel(emp) {
  const code = emp.employeeCode || '';
  const name = emp.name || '';
  if (code && name) {
    return code + ' ' + name;
  }
  return code || name || '-';
}

function updateTimelineHint(message) {
  const hint = document.getElementById('timelineEmployeeHint');
  if (!hint) return;
  hint.textContent = message;
}

function renderTimelineSearchOptions(keyword) {
  const panel = document.getElementById('timelineEmployeePanel');
  if (!panel) return;
  renderEmployeeSearchOptions(panel, keyword);
}

function refreshTimelineSearch() {
  if (!employees || employees.length === 0) {
    updateTimelineHint('暂无员工');
  } else {
    updateTimelineHint('已加载 ' + employees.length + ' 人');
  }
  refreshEmployeeSearchPanel('timelineEmployeePanel', 'timelineEmployeeSearch');
}

function initTimelineSearch() {
  if (timelineSearchReady) return;
  const ready = initEmployeeSearch('timelineEmployeeSelect', 'timelineEmployeeSearch', 'timelineEmployeePanel', 'timelineEmployee', 'timeline');
  if (ready) {
    timelineSearchReady = true;
  }
}

function renderEmployeeOptions() {
  refreshEmployeeSearchHints();
  refreshTimelineSearch();
  refreshEmployeeSearchPanel('adjustEmployeePanel', 'adjustEmployeeSearch');
  refreshEmployeeSearchPanel('offlineEmployeePanel', 'offlineEmployeeSearch');
  refreshEmployeeSearchPanel('checkoutEmployeePanel', 'checkoutEmployeeSearch');
  refreshEmployeeSearchPanel('reviewEmployeePanel', 'reviewEmployeeSearch');
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
  if (!payload.name) {
    setStatus('姓名不能为空', document.getElementById('employeeStatus'));
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
  const headers = ['工号', '姓名', '部门', '绑定状态', '上班时间', '下班时间', '最近上报', '状态', '操作'];
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
    const clockIn = item.lastClockIn || '未上班';
    const clockOut = item.lastClockOut || (item.lastClockIn ? '未下班' : '-');
    return [
      '<div class="table-row cols-9">',
      '<div>' + item.employeeCode + '</div>',
      '<div>' + item.name + '</div>',
      '<div>' + (item.department || '-') + '</div>',
      '<div>' + item.bindStatus + '</div>',
      '<div>' + clockIn + '</div>',
      '<div>' + clockOut + '</div>',
      '<div>' + (item.lastSeen || '-') + '</div>',
      '<div>' + statusLabel + '</div>',
      '<div>' + actions.join('') + '</div>',
      '</div>'
    ].join('');
  });
  renderTable(container, headers, rows, 'cols-9');
}

const attendanceStatusOptions = [
  { code: 'work', label: '工作' },
  { code: 'normal', label: '常规' },
  { code: 'fish', label: '摸鱼' },
  { code: 'idle', label: '离开' },
  { code: 'offline', label: '离线' },
  { code: 'break', label: '休息' },
];

function getStatusLabel(code) {
  const item = attendanceStatusOptions.find((row) => row.code === code);
  return item ? item.label : (code || '-');
}

function getTriggerActionLabel(action) {
  if (action === 'require_reason') {
    return '下班需要补录原因';
  }
  return '仅后台显示';
}

function splitDuration(seconds) {
  const total = Math.max(0, Number(seconds) || 0);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  return { hours: hours, minutes: minutes };
}

function formatDurationText(seconds) {
  const value = Math.max(0, Number(seconds) || 0);
  const { hours, minutes } = splitDuration(value);
  if (hours <= 0 && minutes <= 0) {
    return '0 分钟';
  }
  if (hours > 0 && minutes > 0) {
    return hours + ' 小时 ' + minutes + ' 分钟';
  }
  if (hours > 0) {
    return hours + ' 小时';
  }
  return minutes + ' 分钟';
}

function readDurationSeconds(hoursId, minutesId) {
  const hours = Number(document.getElementById(hoursId).value || 0);
  const minutes = Number(document.getElementById(minutesId).value || 0);
  if (hours < 0 || minutes < 0) {
    return 0;
  }
  return Math.floor(hours * 3600 + minutes * 60);
}

function setDurationInputs(hoursId, minutesId, seconds) {
  const value = Math.max(0, Number(seconds) || 0);
  const { hours, minutes } = splitDuration(value);
  document.getElementById(hoursId).value = hours ? String(hours) : '';
  document.getElementById(minutesId).value = minutes ? String(minutes) : '';
}

function renderEmployeeSearchOptions(panel, keyword) {
  if (!panel) return;
  const value = (keyword || '').trim().toLowerCase();
  if (!value) {
    panel.innerHTML = '<div class="search-select-empty">请输入工号或姓名搜索</div>';
    return;
  }
  const list = employees.filter((emp) => {
    const text = [emp.employeeCode, emp.name].join(' ').toLowerCase();
    return text.includes(value);
  });
  if (list.length === 0) {
    panel.innerHTML = '<div class="search-select-empty">未找到匹配员工</div>';
    return;
  }
  panel.innerHTML = list.map((emp) => {
    return '<div class="search-select-item" data-code="' + emp.employeeCode + '">' + formatEmployeeLabel(emp) + '</div>';
  }).join('');
}

function refreshEmployeeSearchPanel(panelId, inputId) {
  const panel = document.getElementById(panelId);
  const input = document.getElementById(inputId);
  if (panel && panel.classList.contains('is-open')) {
    renderEmployeeSearchOptions(panel, input ? input.value : '');
  }
}

function refreshEmployeeSearchHints() {
  const count = employees.length;
  const timelineHint = document.getElementById('timelineEmployeeHint');
  if (timelineHint) {
    timelineHint.textContent = count > 0 ? ('已加载 ' + count + ' 人') : '暂无员工';
  }
  const adjustHint = document.getElementById('adjustEmployeeHint');
  if (adjustHint) {
    adjustHint.textContent = count > 0 ? '输入工号/姓名搜索' : '暂无员工';
  }
  const offlineHint = document.getElementById('offlineEmployeeHint');
  if (offlineHint) {
    offlineHint.textContent = count > 0 ? '可选，不填则查询全部' : '暂无员工';
  }
  const checkoutHint = document.getElementById('checkoutEmployeeHint');
  if (checkoutHint) {
    checkoutHint.textContent = count > 0 ? '可选，不填则查询全部' : '暂无员工';
  }
  const reviewHint = document.getElementById('reviewEmployeeHint');
  if (reviewHint) {
    reviewHint.textContent = count > 0 ? '可选，不填则查询全部' : '暂无员工';
  }
}

function initEmployeeSearch(rootId, inputId, panelId, hiddenId, readyKey) {
  if (employeeSearchReady[readyKey]) return false;
  const root = document.getElementById(rootId);
  const input = document.getElementById(inputId);
  const panel = document.getElementById(panelId);
  const hidden = document.getElementById(hiddenId);
  if (!root || !input || !panel || !hidden) return false;
  employeeSearchReady[readyKey] = true;

  const openPanel = () => {
    panel.classList.add('is-open');
    renderEmployeeSearchOptions(panel, input.value);
  };

  input.addEventListener('focus', openPanel);
  input.addEventListener('input', () => {
    hidden.value = '';
    openPanel();
  });

  panel.addEventListener('mousedown', (event) => {
    const item = event.target.closest('.search-select-item');
    if (!item) return;
    const code = item.dataset.code || '';
    const emp = employees.find((row) => row.employeeCode === code);
    if (emp) {
      hidden.value = emp.employeeCode;
      input.value = formatEmployeeLabel(emp);
    }
    panel.classList.remove('is-open');
    event.preventDefault();
  });

  document.addEventListener('click', (event) => {
    if (!root.contains(event.target)) {
      panel.classList.remove('is-open');
    }
  });
  return true;
}

function initEmployeeSearches() {
  initTimelineSearch();
  initEmployeeSearch('adjustEmployeeSelect', 'adjustEmployeeSearch', 'adjustEmployeePanel', 'adjustEmployee', 'adjust');
  initEmployeeSearch('offlineEmployeeSelect', 'offlineEmployeeSearch', 'offlineEmployeePanel', 'offlineEmployee', 'offline');
  initEmployeeSearch('checkoutEmployeeSelect', 'checkoutEmployeeSearch', 'checkoutEmployeePanel', 'checkoutEmployee', 'checkout');
  initEmployeeSearch('reviewEmployeeSelect', 'reviewEmployeeSearch', 'reviewEmployeePanel', 'reviewEmployee', 'review');
  refreshEmployeeSearchHints();
}

function resolveEmployeeCode(hiddenValue, inputValue) {
  const hidden = (hiddenValue || '').trim();
  if (hidden) return hidden;
  const typed = (inputValue || '').trim();
  if (!typed) return '';
  const exact = employees.find((emp) => emp.employeeCode === typed);
  if (exact) return exact.employeeCode;
  const guess = typed.split(/\s+/)[0];
  const matched = employees.find((emp) => emp.employeeCode === guess);
  return matched ? matched.employeeCode : '';
}

function getEmployeeKeyword(hiddenValue, inputValue) {
  const hidden = (hiddenValue || '').trim();
  if (hidden) return hidden;
  return (inputValue || '').trim();
}

function setEmployeeSearchValue(hiddenId, inputId, code) {
  const hidden = document.getElementById(hiddenId);
  const input = document.getElementById(inputId);
  if (hidden) hidden.value = code || '';
  if (input) {
    const emp = employees.find((row) => row.employeeCode === code);
    input.value = emp ? formatEmployeeLabel(emp) : (code || '');
  }
}
function renderAttendanceStatusOptions() {
  const select = document.getElementById('attendanceStatusCode');
  if (!select) return;
  select.innerHTML = attendanceStatusOptions.map((item) => '<option value="' + item.code + '">' + item.label + '</option>').join('');
}

async function loadAttendanceRules() {
  const deptId = Number(document.getElementById('attendanceDepartment').value || 0);
  attendanceRules = [];
  attendanceThresholds = [];
  editingAttendanceThresholdIndex = null;
  if (!deptId) {
    resetAttendanceRuleForm();
    resetAttendanceThresholdForm();
    renderAttendanceThresholdTable();
    return;
  }
  try {
    const data = await fetchJSON('/api/v1/admin/department-rules?departmentId=' + deptId);
    fillAttendanceRuleForm(data || {});
    setStatus('规则已加载', document.getElementById('attendanceStatus'));
  } catch (error) {
    setStatus(error.message, document.getElementById('attendanceStatus'));
    resetAttendanceRuleForm();
    renderAttendanceThresholdTable();
  }
}

function resetAttendanceRuleForm() {
  document.getElementById('attendanceEnabled').checked = false;
  setDurationInputs('attendanceTargetHours', 'attendanceTargetMinutes', 0);
  setDurationInputs('attendanceBreakHours', 'attendanceBreakMinutes', 0);
  document.getElementById('attendanceBreakCount').value = '';
  setDurationInputs('attendanceBreakSingleHours', 'attendanceBreakSingleMinutes', 0);
}

function fillAttendanceRuleForm(data) {
  const enabled = !!data.enabled;
  const rule = data.rule || {};
  document.getElementById('attendanceEnabled').checked = enabled;
  setDurationInputs('attendanceTargetHours', 'attendanceTargetMinutes', rule.targetSeconds || 0);
  setDurationInputs('attendanceBreakHours', 'attendanceBreakMinutes', rule.maxBreakSeconds || 0);
  document.getElementById('attendanceBreakCount').value = rule.maxBreakCount || 0;
  setDurationInputs('attendanceBreakSingleHours', 'attendanceBreakSingleMinutes', rule.maxBreakSingleSeconds || 0);
  attendanceThresholds = Array.isArray(data.thresholds) ? data.thresholds : [];
  renderAttendanceThresholdTable();
  resetAttendanceThresholdForm();
}

async function saveAttendanceRule() {
  const deptId = Number(document.getElementById('attendanceDepartment').value || 0);
  if (!deptId) {
    setStatus('请选择部门', document.getElementById('attendanceStatus'));
    return;
  }
  const enabled = document.getElementById('attendanceEnabled').checked;
  const payload = {
    departmentId: deptId,
    enabled: enabled,
    rule: {
      targetSeconds: readDurationSeconds('attendanceTargetHours', 'attendanceTargetMinutes'),
      maxBreakSeconds: readDurationSeconds('attendanceBreakHours', 'attendanceBreakMinutes'),
      maxBreakCount: Number(document.getElementById('attendanceBreakCount').value || 0),
      maxBreakSingleSeconds: readDurationSeconds('attendanceBreakSingleHours', 'attendanceBreakSingleMinutes'),
    },
    thresholds: enabled ? attendanceThresholds : [],
  };
  if (payload.rule.maxBreakCount < 0) {
    setStatus('休息次数不能为负数', document.getElementById('attendanceStatus'));
    return;
  }
  try {
    await fetchJSON('/api/v1/admin/department-rules', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', document.getElementById('attendanceStatus'));
    loadAttendanceRules();
  } catch (error) {
    setStatus(error.message, document.getElementById('attendanceStatus'));
  }
}

function getThresholdOperatorLabel(item) {
  if (item.minSeconds && item.maxSeconds) {
    return '区间';
  }
  if (item.minSeconds) {
    return '小于';
  }
  if (item.maxSeconds) {
    return '大于';
  }
  return '-';
}

function getThresholdLimitText(item) {
  if (item.minSeconds && item.maxSeconds) {
    return formatDurationText(item.minSeconds) + ' ~ ' + formatDurationText(item.maxSeconds);
  }
  const seconds = item.minSeconds || item.maxSeconds || 0;
  return seconds ? formatDurationText(seconds) : '-';
}
function renderAttendanceThresholdTable() {
  const container = document.getElementById('attendanceThresholdTable');
  if (!container) return;
  if (!attendanceThresholds || attendanceThresholds.length === 0) {
    container.innerHTML = '<div class="empty-hint">暂无阈值配置</div>';
    return;
  }
  const headers = ['状态', '阈值方向', '阈值时长', '触发处理', '启用', '操作'];
  const rows = attendanceThresholds.map((item, index) => {
    const operatorLabel = getThresholdOperatorLabel(item);
    const limitText = getThresholdLimitText(item);
    return [
      '<div class="table-row cols-6">',
      '<div>' + getStatusLabel(item.statusCode) + '</div>',
      '<div>' + operatorLabel + '</div>',
      '<div>' + limitText + '</div>',
      '<div>' + getTriggerActionLabel(item.triggerAction) + '</div>',
      '<div>' + (item.enabled ? '是' : '否') + '</div>',
      '<div>',
      '<button class="btn btn-secondary" data-action="edit" data-index="' + index + '">编辑</button>',
      '<button class="btn btn-secondary" data-action="delete" data-index="' + index + '">删除</button>',
      '</div>',
      '</div>'
    ].join('');
  });
  renderTable(container, headers, rows, 'cols-6');
}

function resetAttendanceThresholdForm() {
  editingAttendanceThresholdIndex = null;
  document.getElementById('attendanceStatusCode').value = 'work';
  document.getElementById('attendanceThresholdOperator').value = 'less';
  setDurationInputs('attendanceThresholdHours', 'attendanceThresholdMinutes', 0);
  document.getElementById('attendanceTriggerAction').value = 'show_only';
  document.getElementById('attendanceThresholdEnabled').checked = true;
  document.getElementById('saveAttendanceThreshold').textContent = '保存阈值';
}

function fillAttendanceThresholdForm(index) {
  const item = attendanceThresholds[index];
  if (!item) return;
  editingAttendanceThresholdIndex = index;
  document.getElementById('attendanceStatusCode').value = item.statusCode || 'work';
  const operator = item.minSeconds && !item.maxSeconds ? 'less' : (item.maxSeconds && !item.minSeconds ? 'greater' : (item.minSeconds ? 'less' : 'greater'));
  document.getElementById('attendanceThresholdOperator').value = operator;
  const valueSeconds = operator === 'greater' ? (item.maxSeconds || 0) : (item.minSeconds || 0);
  setDurationInputs('attendanceThresholdHours', 'attendanceThresholdMinutes', valueSeconds);
  document.getElementById('attendanceTriggerAction').value = item.triggerAction || 'show_only';
  document.getElementById('attendanceThresholdEnabled').checked = !!item.enabled;
  document.getElementById('saveAttendanceThreshold').textContent = '更新阈值';
}

function saveAttendanceThreshold() {
  const statusCode = document.getElementById('attendanceStatusCode').value;
  const operator = document.getElementById('attendanceThresholdOperator').value;
  const valueSeconds = readDurationSeconds('attendanceThresholdHours', 'attendanceThresholdMinutes');
  const triggerAction = document.getElementById('attendanceTriggerAction').value;
  const enabled = document.getElementById('attendanceThresholdEnabled').checked;
  if (!valueSeconds) {
    setStatus('请填写阈值时长', document.getElementById('attendanceThresholdStatus'));
    return;
  }
  const minSeconds = operator === 'less' ? valueSeconds : 0;
  const maxSeconds = operator === 'greater' ? valueSeconds : 0;
  const item = {
    statusCode: statusCode,
    minSeconds: minSeconds,
    maxSeconds: maxSeconds,
    triggerAction: triggerAction,
    enabled: enabled,
  };
  if (editingAttendanceThresholdIndex !== null) {
    attendanceThresholds[editingAttendanceThresholdIndex] = item;
  } else {
    const existingIndex = attendanceThresholds.findIndex((row) => row.statusCode === statusCode);
    if (existingIndex >= 0) {
      attendanceThresholds[existingIndex] = item;
    } else {
      attendanceThresholds.push(item);
    }
  }
  renderAttendanceThresholdTable();
  resetAttendanceThresholdForm();
  setStatus('阈值已更新', document.getElementById('attendanceThresholdStatus'));
}

function deleteAttendanceThreshold(index) {
  attendanceThresholds.splice(index, 1);
  renderAttendanceThresholdTable();
  resetAttendanceThresholdForm();
}
async function loadCheckoutTemplates() {
  const deptId = Number(document.getElementById('checkoutDepartment').value || 0);
  const container = document.getElementById('checkoutTemplatesTable');
  if (!deptId) {
    checkoutTemplates = [];
    activeCheckoutTemplateId = null;
    if (container) {
      container.innerHTML = '<div class="empty-hint">请选择部门</div>';
    }
    renderCheckoutFieldsTable([]);
    setStatus('请先选择部门', document.getElementById('checkoutTemplateStatus'));
    return;
  }
  try {
    const items = await fetchJSON('/api/v1/admin/checkout-templates?departmentId=' + deptId);
    checkoutTemplates = Array.isArray(items) ? items : [];
    renderCheckoutTemplatesTable(checkoutTemplates);
    const enabled = checkoutTemplates.find((item) => item.enabled);
    if (enabled) {
      setActiveCheckoutTemplate(enabled.id);
    } else if (checkoutTemplates.length > 0) {
      setActiveCheckoutTemplate(checkoutTemplates[0].id);
    } else {
      activeCheckoutTemplateId = null;
      renderCheckoutFieldsTable([]);
    }
  } catch (error) {
    if (container) {
      container.innerHTML = '<div class="empty-hint">' + error.message + '</div>';
    }
  }
}

function renderCheckoutTemplatesTable(items) {
  const container = document.getElementById('checkoutTemplatesTable');
  if (!container) return;
  if (!items || items.length === 0) {
    container.innerHTML = '<div class="empty-hint">暂无模板</div>';
    return;
  }
  const headers = ['模板名称', '字段数', '启用', '更新时间', '操作'];
  const rows = items.map((item) => {
    const activeTag = item.id === activeCheckoutTemplateId ? '<span class="muted">当前</span>' : '';
    return [
      '<div class="table-row cols-5">',
      '<div>' + item.name + '</div>',
      '<div>' + item.fieldCount + '</div>',
      '<div>' + (item.enabled ? '是' : '否') + '</div>',
      '<div>' + (item.updatedAt || '-') + '</div>',
      '<div>',
      activeTag,
      '<button class="btn btn-secondary" data-action="edit" data-id="' + item.id + '">编辑</button>',
      '<button class="btn btn-secondary" data-action="fields" data-id="' + item.id + '">字段</button>',
      '<button class="btn btn-secondary" data-action="delete" data-id="' + item.id + '">删除</button>',
      '</div>',
      '</div>'
    ].join('');
  });
  renderTable(container, headers, rows, 'cols-5');
}

function resetCheckoutTemplateForm() {
  editingCheckoutTemplateId = null;
  document.getElementById('checkoutTemplateName').value = '';
  document.getElementById('checkoutTemplateEnabled').checked = false;
  document.getElementById('saveCheckoutTemplate').textContent = '保存模板';
}

function fillCheckoutTemplateForm(item) {
  editingCheckoutTemplateId = item.id;
  document.getElementById('checkoutTemplateName').value = item.name;
  document.getElementById('checkoutTemplateEnabled').checked = !!item.enabled;
  document.getElementById('saveCheckoutTemplate').textContent = '更新模板';
}

async function saveCheckoutTemplate() {
  const deptId = Number(document.getElementById('checkoutDepartment').value || 0);
  if (!deptId) {
    setStatus('请选择部门', document.getElementById('checkoutTemplateStatus'));
    return;
  }
  const payload = {
    id: editingCheckoutTemplateId || 0,
    departmentId: deptId,
    name: document.getElementById('checkoutTemplateName').value.trim(),
    enabled: document.getElementById('checkoutTemplateEnabled').checked,
  };
  if (!payload.name) {
    setStatus('模板名称不能为空', document.getElementById('checkoutTemplateStatus'));
    return;
  }
  try {
    await fetchJSON('/api/v1/admin/checkout-templates', {
      method: editingCheckoutTemplateId ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', document.getElementById('checkoutTemplateStatus'));
    resetCheckoutTemplateForm();
    loadCheckoutTemplates();
  } catch (error) {
    setStatus(error.message, document.getElementById('checkoutTemplateStatus'));
  }
}

async function deleteCheckoutTemplate(id) {
  try {
    await fetchJSON('/api/v1/admin/checkout-templates?id=' + id, { method: 'DELETE' });
    if (activeCheckoutTemplateId === id) {
      activeCheckoutTemplateId = null;
    }
    loadCheckoutTemplates();
  } catch (error) {
    setStatus(error.message, document.getElementById('checkoutTemplateStatus'));
  }
}

function setActiveCheckoutTemplate(id) {
  activeCheckoutTemplateId = id || null;
  const target = checkoutTemplates.find((item) => item.id === activeCheckoutTemplateId);
  if (target) {
    document.getElementById('checkoutFieldStatus').textContent = '当前模板：' + target.name;
    loadCheckoutFields();
  } else {
    document.getElementById('checkoutFieldStatus').textContent = '请选择模板';
    renderCheckoutFieldsTable([]);
  }
}

async function loadCheckoutFields() {
  const container = document.getElementById('checkoutFieldsTable');
  if (!activeCheckoutTemplateId) {
    if (container) {
      container.innerHTML = '<div class="empty-hint">请选择模板</div>';
    }
    return;
  }
  try {
    const items = await fetchJSON('/api/v1/admin/checkout-fields?templateId=' + activeCheckoutTemplateId);
    checkoutFields = Array.isArray(items) ? items : [];
    renderCheckoutFieldsTable(checkoutFields);
  } catch (error) {
    if (container) {
      container.innerHTML = '<div class="empty-hint">' + error.message + '</div>';
    }
  }
}

function renderCheckoutFieldsTable(items) {
  const container = document.getElementById('checkoutFieldsTable');
  if (!container) return;
  if (!items || items.length === 0) {
    container.innerHTML = '<div class="empty-hint">暂无字段</div>';
    return;
  }
  const headers = ['字段名称', '类型', '必填', '启用', '排序', '选项', '操作'];
  const rows = items.map((item) => {
    const typeLabel = item.type === 'number' ? '数字' : (item.type === 'select' ? '下拉单选' : '文本');
    const options = item.options && item.options.length > 0 ? item.options.join('、') : '-';
    return [
      '<div class="table-row cols-7">',
      '<div>' + item.name + '</div>',
      '<div>' + typeLabel + '</div>',
      '<div>' + (item.required ? '是' : '否') + '</div>',
      '<div>' + (item.enabled ? '是' : '否') + '</div>',
      '<div>' + item.sortOrder + '</div>',
      '<div>' + options + '</div>',
      '<div>',
      '<button class="btn btn-secondary" data-action="edit" data-id="' + item.id + '">编辑</button>',
      '<button class="btn btn-secondary" data-action="delete" data-id="' + item.id + '">删除</button>',
      '</div>',
      '</div>'
    ].join('');
  });
  renderTable(container, headers, rows, 'cols-7');
}

function resetCheckoutFieldForm() {
  editingCheckoutFieldId = null;
  document.getElementById('checkoutFieldName').value = '';
  document.getElementById('checkoutFieldType').value = 'text';
  document.getElementById('checkoutFieldSort').value = '0';
  document.getElementById('checkoutFieldOptions').value = '';
  document.getElementById('checkoutFieldRequired').checked = false;
  document.getElementById('checkoutFieldEnabled').checked = true;
  document.getElementById('saveCheckoutField').textContent = '保存字段';
  toggleCheckoutFieldOptions();
}

function fillCheckoutFieldForm(item) {
  editingCheckoutFieldId = item.id;
  document.getElementById('checkoutFieldName').value = item.name;
  document.getElementById('checkoutFieldType').value = item.type;
  document.getElementById('checkoutFieldSort').value = item.sortOrder || 0;
  document.getElementById('checkoutFieldOptions').value = (item.options || []).join('\n');
  document.getElementById('checkoutFieldRequired').checked = !!item.required;
  document.getElementById('checkoutFieldEnabled').checked = !!item.enabled;
  document.getElementById('saveCheckoutField').textContent = '更新字段';
  toggleCheckoutFieldOptions();
}

function parseCheckoutFieldOptions() {
  const raw = document.getElementById('checkoutFieldOptions').value || '';
  return raw.split(/\r?\n/).map((item) => item.trim()).filter((item) => item);
}

function toggleCheckoutFieldOptions() {
  const type = document.getElementById('checkoutFieldType').value;
  const options = document.getElementById('checkoutFieldOptions');
  if (!options) return;
  if (type === 'select') {
    options.removeAttribute('disabled');
  } else {
    options.value = '';
    options.setAttribute('disabled', 'disabled');
  }
}

async function saveCheckoutField() {
  if (!activeCheckoutTemplateId) {
    setStatus('请先选择模板', document.getElementById('checkoutFieldStatus'));
    return;
  }
  const payload = {
    id: editingCheckoutFieldId || 0,
    templateId: activeCheckoutTemplateId,
    name: document.getElementById('checkoutFieldName').value.trim(),
    type: document.getElementById('checkoutFieldType').value,
    required: document.getElementById('checkoutFieldRequired').checked,
    sortOrder: Number(document.getElementById('checkoutFieldSort').value || 0),
    enabled: document.getElementById('checkoutFieldEnabled').checked,
    options: [],
  };
  if (!payload.name) {
    setStatus('字段名称不能为空', document.getElementById('checkoutFieldStatus'));
    return;
  }
  if (payload.type === 'select') {
    payload.options = parseCheckoutFieldOptions();
    if (payload.options.length === 0) {
      setStatus('请填写下拉选项', document.getElementById('checkoutFieldStatus'));
      return;
    }
  }
  try {
    await fetchJSON('/api/v1/admin/checkout-fields', {
      method: editingCheckoutFieldId ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', document.getElementById('checkoutFieldStatus'));
    resetCheckoutFieldForm();
    loadCheckoutFields();
    loadCheckoutTemplates();
  } catch (error) {
    setStatus(error.message, document.getElementById('checkoutFieldStatus'));
  }
}

async function deleteCheckoutField(id) {
  try {
    await fetchJSON('/api/v1/admin/checkout-fields?id=' + id, { method: 'DELETE' });
    loadCheckoutFields();
    loadCheckoutTemplates();
  } catch (error) {
    setStatus(error.message, document.getElementById('checkoutFieldStatus'));
  }
}
async function loadCheckoutTemplatesForFilter() {
  const select = document.getElementById('checkoutQueryTemplate');
  const deptId = Number(document.getElementById('checkoutQueryDepartment').value || 0);
  if (!select) return;
  if (!deptId) {
    select.innerHTML = '<option value="0">全部</option>';
    return;
  }
  try {
    const items = await fetchJSON('/api/v1/admin/checkout-templates?departmentId=' + deptId);
    const options = (items || []).map((item) => '<option value="' + item.id + '">' + item.name + '</option>').join('');
    select.innerHTML = '<option value="0">全部</option>' + options;
  } catch (error) {
    select.innerHTML = '<option value="0">全部</option>';
  }
}

function applyCheckoutQuickRange(rangeKey) {
  const end = new Date();
  let start = new Date();
  if (rangeKey === 'yesterday') {
    start.setDate(start.getDate() - 1);
    end.setDate(end.getDate() - 1);
  } else if (rangeKey === 'week') {
    const day = start.getDay();
    const diff = day === 0 ? 6 : day - 1;
    start.setDate(start.getDate() - diff);
  } else if (rangeKey === 'month') {
    start = new Date(start.getFullYear(), start.getMonth(), 1);
  }
  document.getElementById('checkoutQueryStart').value = start.toISOString().slice(0, 10);
  document.getElementById('checkoutQueryEnd').value = end.toISOString().slice(0, 10);
  loadCheckoutRecords(1);
}

function resetCheckoutQuery() {
  const today = new Date().toISOString().slice(0, 10);
  document.getElementById('checkoutQueryStart').value = today;
  document.getElementById('checkoutQueryEnd').value = today;
  document.getElementById('checkoutQueryDepartment').value = '0';
  document.getElementById('checkoutQueryTemplate').innerHTML = '<option value="0">全部</option>';
  setEmployeeSearchValue('checkoutEmployee', 'checkoutEmployeeSearch', '');
  loadCheckoutRecords(1);
}

async function loadCheckoutRecords(page) {
  const startDate = document.getElementById('checkoutQueryStart').value || new Date().toISOString().slice(0, 10);
  const endDate = document.getElementById('checkoutQueryEnd').value || new Date().toISOString().slice(0, 10);
  const deptId = Number(document.getElementById('checkoutQueryDepartment').value || 0);
  const templateId = Number(document.getElementById('checkoutQueryTemplate').value || 0);
  const keyword = getEmployeeKeyword(document.getElementById('checkoutEmployee').value, document.getElementById('checkoutEmployeeSearch').value);
  const pageSize = 20;
  const currentPage = page || checkoutRecordsPage || 1;
  let url = '/api/v1/admin/checkout-records?startDate=' + encodeURIComponent(startDate) + '&endDate=' + encodeURIComponent(endDate);
  if (deptId) {
    url += '&departmentId=' + deptId;
  }
  if (templateId) {
    url += '&templateId=' + templateId;
  }
  if (keyword) {
    url += '&employeeKeyword=' + encodeURIComponent(keyword);
  }
  url += '&page=' + currentPage + '&pageSize=' + pageSize;
  try {
    const data = await fetchJSON(url);
    checkoutRecordsCache = data.items || [];
    checkoutRecordsTotal = data.total || 0;
    checkoutRecordsPage = data.page || currentPage;
    renderCheckoutRecordsTable(checkoutRecordsCache);
    renderCheckoutPager(checkoutRecordsTotal, checkoutRecordsPage, pageSize);
  } catch (error) {
    document.getElementById('checkoutRecordsTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
    document.getElementById('checkoutRecordsPager').innerHTML = '';
  }
}

function renderCheckoutRecordsTable(items) {
  const container = document.getElementById('checkoutRecordsTable');
  if (!container) return;
  if (!items || items.length === 0) {
    container.innerHTML = '<div class="empty-hint">暂无记录</div>';
    return;
  }
  const headers = ['工号', '姓名', '部门', '上班', '下班', '模板', '录入摘要', '录入时间', '操作'];
  const rows = items.map((item) => [
    '<div class="table-row cols-9">',
    '<div>' + item.employeeCode + '</div>',
    '<div>' + item.name + '</div>',
    '<div>' + (item.department || '-') + '</div>',
    '<div>' + item.startAt + '</div>',
    '<div>' + (item.endAt || '-') + '</div>',
    '<div>' + (item.templateName || '-') + '</div>',
    '<div>' + (item.summary || '-') + '</div>',
    '<div>' + (item.createdAt || '-') + '</div>',
    '<div><button class="btn btn-secondary" data-action="detail" data-id="' + item.id + '">详情</button></div>',
    '</div>'
  ].join(''));
  renderTable(container, headers, rows, 'cols-9');
}

function renderCheckoutPager(total, page, pageSize) {
  const pager = document.getElementById('checkoutRecordsPager');
  if (!pager) return;
  pager.innerHTML = buildPagerHtml(total, page, pageSize);
}

async function openCheckoutDetail(id) {
  const modal = document.getElementById('checkoutDetailModal');
  const meta = document.getElementById('checkoutDetailMeta');
  const fields = document.getElementById('checkoutDetailFields');
  if (modal) modal.classList.add('is-active');
  if (meta) meta.innerHTML = '<div class="empty-hint">加载中...</div>';
  if (fields) fields.innerHTML = '';
  try {
    const data = await fetchJSON('/api/v1/admin/checkout-record?id=' + id);
    renderCheckoutDetail(data);
  } catch (error) {
    if (meta) meta.innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderCheckoutDetail(detail) {
  const meta = document.getElementById('checkoutDetailMeta');
  const fields = document.getElementById('checkoutDetailFields');
  if (!meta || !fields) return;
  const metaItems = [
    { label: '工号', value: detail.employeeCode },
    { label: '姓名', value: detail.name },
    { label: '部门', value: detail.department || '-' },
    { label: '上班时间', value: detail.startAt || '-' },
    { label: '下班时间', value: detail.endAt || '-' },
    { label: '模板', value: detail.templateName || '-' },
    { label: '录入时间', value: detail.createdAt || '-' },
  ];
  meta.innerHTML = metaItems.map((item) => {
    return '<div class="detail-meta-item"><span>' + item.label + '</span><strong>' + item.value + '</strong></div>';
  }).join('');
  if (!detail.fields || detail.fields.length === 0) {
    fields.innerHTML = '<div class="empty-hint">暂无录入字段</div>';
    return;
  }
  fields.innerHTML = detail.fields.map((field) => {
    return '<div class="detail-field"><div class="detail-field-name">' + field.name + '</div><div class="detail-field-value">' + (field.value || '-') + '</div></div>';
  }).join('');
}

function closeCheckoutDetail() {
  const modal = document.getElementById('checkoutDetailModal');
  if (modal) modal.classList.remove('is-active');
}
async function loadReviewList(page) {
  const startDate = document.getElementById('reviewStartDate').value || new Date().toISOString().slice(0, 10);
  const endDate = document.getElementById('reviewEndDate').value || new Date().toISOString().slice(0, 10);
  const deptId = Number(document.getElementById('reviewDepartment').value || 0);
  const keyword = getEmployeeKeyword(document.getElementById('reviewEmployee').value, document.getElementById('reviewEmployeeSearch').value);
  const pageSize = 20;
  const currentPage = page || reviewPage || 1;
  let url = '/api/v1/admin/work-session-reviews?startDate=' + encodeURIComponent(startDate) + '&endDate=' + encodeURIComponent(endDate);
  if (deptId) {
    url += '&departmentId=' + deptId;
  }
  if (keyword) {
    url += '&employeeKeyword=' + encodeURIComponent(keyword);
  }
  url += '&page=' + currentPage + '&pageSize=' + pageSize;
  try {
    const data = await fetchJSON(url);
    checkoutReviewCache = data.items || [];
    checkoutReviewTotal = data.total || 0;
    reviewPage = currentPage;
    renderReviewTable(checkoutReviewCache);
    renderReviewPager(checkoutReviewTotal, reviewPage, pageSize);
  } catch (error) {
    document.getElementById('reviewTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
    document.getElementById('reviewPager').innerHTML = '';
  }
}

function resetReviewList() {
  const today = new Date().toISOString().slice(0, 10);
  document.getElementById('reviewStartDate').value = today;
  document.getElementById('reviewEndDate').value = today;
  document.getElementById('reviewDepartment').value = '0';
  setEmployeeSearchValue('reviewEmployee', 'reviewEmployeeSearch', '');
  loadReviewList(1);
}

function renderReviewTable(items) {
  const container = document.getElementById('reviewTable');
  if (!container) return;
  if (!items || items.length === 0) {
    container.innerHTML = '<div class="empty-hint">暂无记录</div>';
    return;
  }
  const headers = ['日期', '工号', '姓名', '部门', '上班', '下班', '工时标准', '触发项', '补录状态', '操作'];
  const rows = items.map((item) => [
    '<div class="table-row cols-10">',
    '<div>' + item.workDate + '</div>',
    '<div>' + item.employeeCode + '</div>',
    '<div>' + item.name + '</div>',
    '<div>' + (item.department || '-') + '</div>',
    '<div>' + (item.startAt || '-') + '</div>',
    '<div>' + (item.endAt || '-') + '</div>',
    '<div>' + (item.workStandard || '-') + '</div>',
    '<div>' + (item.violationSummary || '-') + '</div>',
    '<div>' + (item.reasonStatus || '-') + '</div>',
    '<div><button class="btn btn-secondary" data-action="detail" data-id="' + item.id + '">详情</button></div>',
    '</div>'
  ].join(''));
  renderTable(container, headers, rows, 'cols-10');
}

function renderReviewPager(total, page, pageSize) {
  const pager = document.getElementById('reviewPager');
  if (!pager) return;
  pager.innerHTML = buildPagerHtml(total, page, pageSize);
}

async function openReviewDetail(id) {
  const modal = document.getElementById('reviewDetailModal');
  const meta = document.getElementById('reviewDetailMeta');
  const fields = document.getElementById('reviewDetailFields');
  if (modal) modal.classList.add('is-active');
  if (meta) meta.innerHTML = '<div class="empty-hint">加载中...</div>';
  if (fields) fields.innerHTML = '';
  try {
    const data = await fetchJSON('/api/v1/admin/work-session-review?id=' + id);
    renderReviewDetail(data);
  } catch (error) {
    if (meta) meta.innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderReviewDetail(detail) {
  const meta = document.getElementById('reviewDetailMeta');
  const fields = document.getElementById('reviewDetailFields');
  if (!meta || !fields) return;
  const metaItems = [
    { label: '日期', value: detail.workDate },
    { label: '工号', value: detail.employeeCode },
    { label: '姓名', value: detail.name },
    { label: '部门', value: detail.department || '-' },
    { label: '上班时间', value: detail.startAt || '-' },
    { label: '下班时间', value: detail.endAt || '-' },
    { label: '工时标准', value: detail.workStandard || '-' },
    { label: '休息时长', value: detail.breakDuration || '-' },
    { label: '补录状态', value: detail.reasonStatus || '-' },
  ];
  meta.innerHTML = metaItems.map((item) => '<div class="detail-meta-item"><span>' + item.label + '</span><strong>' + item.value + '</strong></div>').join('');

  const blocks = [];
  if (detail.reason) {
    blocks.push('<div class="detail-field"><div class="detail-field-name">补录原因</div><div class="detail-field-value">' + detail.reason + '</div></div>');
  }
  if (detail.violations && detail.violations.length > 0) {
    blocks.push('<div class="detail-field"><div class="detail-field-name">触发项</div><div class="detail-field-value">' + detail.violations.map((row) => row.message || '-').join('<br/>') + '</div></div>');
  }
  if (detail.statusTotals) {
    const totals = Object.keys(detail.statusTotals).map((key) => {
      return getStatusLabel(key) + '：' + detail.statusTotals[key];
    }).join('<br/>');
    blocks.push('<div class="detail-field"><div class="detail-field-name">状态时长</div><div class="detail-field-value">' + (totals || '-') + '</div></div>');
  }
  fields.innerHTML = blocks.length > 0 ? blocks.join('') : '<div class="empty-hint">暂无详情</div>';
}

function closeReviewDetail() {
  const modal = document.getElementById('reviewDetailModal');
  if (modal) modal.classList.remove('is-active');
}

function buildPagerHtml(total, page, pageSize) {
  if (!total || total <= pageSize) return '';
  const totalPages = Math.ceil(total / pageSize);
  const current = page || 1;
  const start = Math.max(1, current - 2);
  const end = Math.min(totalPages, current + 2);
  const items = [];
  if (current > 1) {
    items.push('<button class="btn btn-secondary" data-page="' + (current - 1) + '">上一页</button>');
  }
  for (let i = start; i <= end; i += 1) {
    const cls = i === current ? 'btn btn-primary' : 'btn btn-secondary';
    items.push('<button class="' + cls + '" data-page="' + i + '">' + i + '</button>');
  }
  if (current < totalPages) {
    items.push('<button class="btn btn-secondary" data-page="' + (current + 1) + '">下一页</button>');
  }
  return items.join('');
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
  renderAttendanceStatusOptions();
  await loadAttendanceRules();
  await loadCheckoutTemplates();
  await loadCheckoutTemplatesForFilter();
  await loadCheckoutRecords(1);
  await loadReviewList(1);
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

const timelineTable = document.getElementById('timelineTable');
if (timelineTable) {
  timelineTable.addEventListener('click', (event) => {
    const btn = event.target.closest('.timeline-toggle');
    if (!btn) return;
    const group = btn.dataset.group;
    if (group === undefined || group === null) return;
    const rows = timelineTable.querySelectorAll('.timeline-child[data-parent="' + group + '"]');
    if (!rows || rows.length === 0) return;
    const shouldShow = rows[0].style.display === 'none';
    rows.forEach((row) => {
      row.style.display = shouldShow ? '' : 'none';
    });
    btn.textContent = shouldShow ? '收起' : '展开';
  });
}
document.getElementById('loadRank').addEventListener('click', loadRank);

document.getElementById('createAdjustment').addEventListener('click', submitAdjustment);
document.getElementById('cancelAdjustment').addEventListener('click', resetAdjustmentForm);

document.getElementById('createIncident').addEventListener('click', submitIncident);
document.getElementById('cancelIncident').addEventListener('click', resetIncidentForm);

document.getElementById('loadOfflineSegments').addEventListener('click', loadOfflineSegments);
document.getElementById('loadAudit').addEventListener('click', loadAuditLogs);

document.getElementById('attendanceDepartment').addEventListener('change', loadAttendanceRules);
document.getElementById('saveAttendanceRule').addEventListener('click', saveAttendanceRule);
document.getElementById('saveAttendanceThreshold').addEventListener('click', saveAttendanceThreshold);
document.getElementById('resetAttendanceThreshold').addEventListener('click', resetAttendanceThresholdForm);
document.getElementById('loadReviewList').addEventListener('click', () => loadReviewList(1));
document.getElementById('resetReviewList').addEventListener('click', resetReviewList);
document.getElementById('closeReviewDetail').addEventListener('click', closeReviewDetail);

document.getElementById('checkoutDepartment').addEventListener('change', () => {
  resetCheckoutTemplateForm();
  loadCheckoutTemplates();
});
document.getElementById('saveCheckoutTemplate').addEventListener('click', saveCheckoutTemplate);
document.getElementById('cancelCheckoutTemplate').addEventListener('click', resetCheckoutTemplateForm);
document.getElementById('saveCheckoutField').addEventListener('click', saveCheckoutField);
document.getElementById('cancelCheckoutField').addEventListener('click', resetCheckoutFieldForm);
document.getElementById('checkoutFieldType').addEventListener('change', toggleCheckoutFieldOptions);

document.getElementById('checkoutQuery').addEventListener('click', () => loadCheckoutRecords(1));
document.getElementById('checkoutReset').addEventListener('click', resetCheckoutQuery);
document.getElementById('checkoutQueryDepartment').addEventListener('change', loadCheckoutTemplatesForFilter);
document.getElementById('checkoutQuickRange').addEventListener('click', (event) => {
  const btn = event.target.closest('button');
  if (!btn) return;
  applyCheckoutQuickRange(btn.dataset.range);
});
document.getElementById('closeCheckoutDetail').addEventListener('click', closeCheckoutDetail);

const attendanceThresholdTable = document.getElementById('attendanceThresholdTable');
if (attendanceThresholdTable) {
  attendanceThresholdTable.addEventListener('click', (event) => {
    const btn = event.target.closest('button');
    if (!btn) return;
    const action = btn.dataset.action;
    const index = Number(btn.dataset.index || -1);
    if (index < 0) return;
    if (action === 'edit') {
      fillAttendanceThresholdForm(index);
    }
    if (action === 'delete') {
      deleteAttendanceThreshold(index);
    }
  });
}

const checkoutTemplatesTable = document.getElementById('checkoutTemplatesTable');
if (checkoutTemplatesTable) {
  checkoutTemplatesTable.addEventListener('click', (event) => {
    const btn = event.target.closest('button');
    if (!btn) return;
    const action = btn.dataset.action;
    const id = Number(btn.dataset.id || 0);
    if (!id) return;
    if (action === 'edit') {
      const item = checkoutTemplates.find((row) => row.id === id);
      if (item) fillCheckoutTemplateForm(item);
    }
    if (action === 'fields') {
      setActiveCheckoutTemplate(id);
    }
    if (action === 'delete') {
      deleteCheckoutTemplate(id);
    }
  });
}

const checkoutFieldsTable = document.getElementById('checkoutFieldsTable');
if (checkoutFieldsTable) {
  checkoutFieldsTable.addEventListener('click', (event) => {
    const btn = event.target.closest('button');
    if (!btn) return;
    const action = btn.dataset.action;
    const id = Number(btn.dataset.id || 0);
    if (!id) return;
    if (action === 'edit') {
      const item = checkoutFields.find((row) => row.id === id);
      if (item) fillCheckoutFieldForm(item);
    }
    if (action === 'delete') {
      deleteCheckoutField(id);
    }
  });
}

const checkoutRecordsTable = document.getElementById('checkoutRecordsTable');
if (checkoutRecordsTable) {
  checkoutRecordsTable.addEventListener('click', (event) => {
    const btn = event.target.closest('button');
    if (!btn) return;
    if (btn.dataset.action === 'detail') {
      const id = Number(btn.dataset.id || 0);
      if (id) openCheckoutDetail(id);
    }
  });
}

const reviewTable = document.getElementById('reviewTable');
if (reviewTable) {
  reviewTable.addEventListener('click', (event) => {
    const btn = event.target.closest('button');
    if (!btn) return;
    if (btn.dataset.action === 'detail') {
      const id = Number(btn.dataset.id || 0);
      if (id) openReviewDetail(id);
    }
  });
}

const checkoutPager = document.getElementById('checkoutRecordsPager');
if (checkoutPager) {
  checkoutPager.addEventListener('click', (event) => {
    const btn = event.target.closest('button');
    if (!btn) return;
    const page = Number(btn.dataset.page || 0);
    if (page) loadCheckoutRecords(page);
  });
}

const reviewPager = document.getElementById('reviewPager');
if (reviewPager) {
  reviewPager.addEventListener('click', (event) => {
    const btn = event.target.closest('button');
    if (!btn) return;
    const page = Number(btn.dataset.page || 0);
    if (page) loadReviewList(page);
  });
}

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












