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

function renderAttendanceThresholdTable() {
  const container = document.getElementById('attendanceThresholdTable');
  if (!container) return;
  if (!attendanceThresholds || attendanceThresholds.length === 0) {
    container.innerHTML = '<div class="empty-hint">暂无阈值配置</div>';
    return;
  }
  const headers = ['状态', '最小阈值', '最大阈值', '触发处理', '启用', '操作'];
  const rows = attendanceThresholds.map((item, index) => [
    '<div class="table-row cols-6">',
    '<div>' + getStatusLabel(item.statusCode) + '</div>',
    '<div>' + (item.minSeconds ? formatDurationText(item.minSeconds) : '-') + '</div>',
    '<div>' + (item.maxSeconds ? formatDurationText(item.maxSeconds) : '-') + '</div>',
    '<div>' + getTriggerActionLabel(item.triggerAction) + '</div>',
    '<div>' + (item.enabled ? '是' : '否') + '</div>',
    '<div>',
    '<button class="btn btn-secondary" data-action="edit" data-index="' + index + '">编辑</button>',
    '<button class="btn btn-secondary" data-action="delete" data-index="' + index + '">删除</button>',
    '</div>',
    '</div>'
  ].join(''));
  renderTable(container, headers, rows, 'cols-6');
}

function resetAttendanceThresholdForm() {
  editingAttendanceThresholdIndex = null;
  document.getElementById('attendanceStatusCode').value = 'work';
  setDurationInputs('attendanceMinHours', 'attendanceMinMinutes', 0);
  setDurationInputs('attendanceMaxHours', 'attendanceMaxMinutes', 0);
  document.getElementById('attendanceTriggerAction').value = 'show_only';
  document.getElementById('attendanceThresholdEnabled').checked = true;
  document.getElementById('saveAttendanceThreshold').textContent = '保存阈值';
}

function fillAttendanceThresholdForm(index) {
  const item = attendanceThresholds[index];
  if (!item) return;
  editingAttendanceThresholdIndex = index;
  document.getElementById('attendanceStatusCode').value = item.statusCode || 'work';
  setDurationInputs('attendanceMinHours', 'attendanceMinMinutes', item.minSeconds || 0);
  setDurationInputs('attendanceMaxHours', 'attendanceMaxMinutes', item.maxSeconds || 0);
  document.getElementById('attendanceTriggerAction').value = item.triggerAction || 'show_only';
  document.getElementById('attendanceThresholdEnabled').checked = !!item.enabled;
  document.getElementById('saveAttendanceThreshold').textContent = '更新阈值';
}

function saveAttendanceThreshold() {
  const statusCode = document.getElementById('attendanceStatusCode').value;
  const minSeconds = readDurationSeconds('attendanceMinHours', 'attendanceMinMinutes');
  const maxSeconds = readDurationSeconds('attendanceMaxHours', 'attendanceMaxMinutes');
  const triggerAction = document.getElementById('attendanceTriggerAction').value;
  const enabled = document.getElementById('attendanceThresholdEnabled').checked;
  if (!minSeconds && !maxSeconds) {
    setStatus('请至少填写最小或最大阈值', document.getElementById('attendanceThresholdStatus'));
    return;
  }
  if (minSeconds && maxSeconds && minSeconds > maxSeconds) {
    setStatus('最小阈值不能大于最大阈值', document.getElementById('attendanceThresholdStatus'));
    return;
  }
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
