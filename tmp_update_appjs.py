from pathlib import Path
import re

path = Path(r"internal/web/static/assets/app.js")
content = path.read_text(encoding="utf-8")
block = Path(r"tmp_block.js").read_text(encoding="utf-8")

marker = "let incidentsCache = [];\n"
insert = marker + "\n" + "\n".join([
    "let attendanceRules = [];",
    "let attendanceThresholds = [];",
    "let editingAttendanceThresholdIndex = null;",
    "let checkoutTemplates = [];",
    "let checkoutFields = [];",
    "let checkoutRecordsCache = [];",
    "let checkoutRecordsTotal = 0;",
    "let checkoutRecordsPage = 1;",
    "let checkoutReviewCache = [];",
    "let checkoutReviewTotal = 0;",
    "let reviewPage = 1;",
    "let editingCheckoutTemplateId = null;",
    "let editingCheckoutFieldId = null;",
    "let activeCheckoutTemplateId = null;",
    "const employeeSearchReady = {};",
]) + "\n\n"
if marker in content:
    content = content.replace(marker, insert, 1)
else:
    raise SystemExit("marker for globals not found")

content = content.replace("['reportDate', 'timelineDate', 'offlineDate', 'auditDate']",
                          "['reportDate', 'timelineDate', 'offlineDate', 'auditDate', 'checkoutQueryStart', 'checkoutQueryEnd', 'reviewStartDate', 'reviewEndDate']")

def sub(pattern, replacement, label):
    global content
    content, n = re.subn(pattern, replacement, content, flags=re.S)
    if n == 0:
        raise SystemExit(label + " not found")

sub(
    r"function renderTimelineSearchOptions\(keyword\)[\s\S]*?function refreshTimelineSearch",
    """function renderTimelineSearchOptions(keyword) {\n  const panel = document.getElementById('timelineEmployeePanel');\n  if (!panel) return;\n  renderEmployeeSearchOptions(panel, keyword);\n}\n\nfunction refreshTimelineSearch""",
    "renderTimelineSearchOptions"
)

sub(
    r"function refreshTimelineSearch\(\)[\s\S]*?function initTimelineSearch",
    """function refreshTimelineSearch() {\n  if (!employees || employees.length === 0) {\n    updateTimelineHint('暂无员工');\n  } else {\n    updateTimelineHint('已加载 ' + employees.length + ' 人');\n  }\n  refreshEmployeeSearchPanel('timelineEmployeePanel', 'timelineEmployeeSearch');\n}\n\nfunction initTimelineSearch""",
    "refreshTimelineSearch"
)

sub(
    r"function initTimelineSearch\(\)[\s\S]*?function renderEmployeeOptions",
    """function initTimelineSearch() {\n  if (timelineSearchReady) return;\n  const ready = initEmployeeSearch('timelineEmployeeSelect', 'timelineEmployeeSearch', 'timelineEmployeePanel', 'timelineEmployee', 'timeline');\n  if (ready) {\n    timelineSearchReady = true;\n  }\n}\n\nfunction renderEmployeeOptions""",
    "initTimelineSearch"
)

sub(
    r"function renderEmployeeOptions\(\)[\s\S]*?function resetEmployeeForm",
    """function renderEmployeeOptions() {\n  refreshEmployeeSearchHints();\n  refreshTimelineSearch();\n  refreshEmployeeSearchPanel('adjustEmployeePanel', 'adjustEmployeeSearch');\n  refreshEmployeeSearchPanel('offlineEmployeePanel', 'offlineEmployeeSearch');\n  refreshEmployeeSearchPanel('checkoutEmployeePanel', 'checkoutEmployeeSearch');\n  refreshEmployeeSearchPanel('reviewEmployeePanel', 'reviewEmployeeSearch');\n}\n\nfunction resetEmployeeForm""",
    "renderEmployeeOptions"
)

sub(
    r"function renderDepartmentOptions\(\)[\s\S]*?function resetDepartmentForm",
    """function renderDepartmentOptions() {\n  const parentSelect = document.getElementById('deptParent');\n  const reportSelect = document.getElementById('reportDepartment');\n  const employeeSelect = document.getElementById('employeeDepartment');\n  const attendanceSelect = document.getElementById('attendanceDepartment');\n  const reviewSelect = document.getElementById('reviewDepartment');\n  const checkoutSelect = document.getElementById('checkoutDepartment');\n  const checkoutQuerySelect = document.getElementById('checkoutQueryDepartment');\n  const options = departments.map((dept) => '<option value="' + dept.id + '">' + dept.name + '</option>').join('');\n  if (parentSelect) {\n    parentSelect.innerHTML = '<option value="0">无</option>' + options;\n  }\n  if (reportSelect) {\n    reportSelect.innerHTML = '<option value="0">全部</option>' + options;\n  }\n  if (employeeSelect) {\n    employeeSelect.innerHTML = '<option value="0">未分配</option>' + options;\n  }\n  if (attendanceSelect) {\n    attendanceSelect.innerHTML = '<option value="0">请选择部门</option>' + options;\n  }\n  if (reviewSelect) {\n    reviewSelect.innerHTML = '<option value="0">全部</option>' + options;\n  }\n  if (checkoutSelect) {\n    checkoutSelect.innerHTML = '<option value="0">请选择部门</option>' + options;\n  }\n  if (checkoutQuerySelect) {\n    checkoutQuerySelect.innerHTML = '<option value="0">全部</option>' + options;\n  }\n}\n\nfunction resetDepartmentForm""",
    "renderDepartmentOptions"
)

content = content.replace(
    "    renderEmployeeOptions();\n    renderEmployeesTable(employees);\n    initTimelineSearch();",
    "    renderEmployeeOptions();\n    renderEmployeesTable(employees);\n    initEmployeeSearches();"
)

sub(
    r"function resetAdjustmentForm\(\)[\s\S]*?function fillAdjustmentForm",
    """function resetAdjustmentForm() {\n  editingAdjustmentId = null;\n  setEmployeeSearchValue('adjustEmployee', 'adjustEmployeeSearch', '');\n  document.getElementById('adjustStart').value = '';\n  document.getElementById('adjustEnd').value = '';\n  document.getElementById('adjustReason').value = '停电';\n  document.getElementById('adjustNote').value = '';\n  document.getElementById('createAdjustment').textContent = '提交补录';\n}\n\nfunction fillAdjustmentForm""",
    "resetAdjustmentForm"
)

sub(
    r"function fillAdjustmentForm\(item\)[\s\S]*?async function submitAdjustment",
    """function fillAdjustmentForm(item) {\n  editingAdjustmentId = item.id;\n  setEmployeeSearchValue('adjustEmployee', 'adjustEmployeeSearch', item.employeeCode);\n  document.getElementById('adjustStart').value = formatDateTimeLocal(item.startAt);\n  document.getElementById('adjustEnd').value = formatDateTimeLocal(item.endAt);\n  document.getElementById('adjustReason').value = item.reason;\n  document.getElementById('adjustNote').value = item.note;\n  document.getElementById('createAdjustment').textContent = '更新补录';\n}\n\nasync function submitAdjustment""",
    "fillAdjustmentForm"
)

sub(
    r"async function submitAdjustment\(\)[\s\S]*?async function revokeAdjustment",
    """async function submitAdjustment() {\n  const hidden = document.getElementById('adjustEmployee').value;\n  const typed = document.getElementById('adjustEmployeeSearch').value;\n  const code = resolveEmployeeCode(hidden, typed);\n  const payload = {\n    id: editingAdjustmentId || 0,\n    employeeCode: code,\n    startAt: document.getElementById('adjustStart').value,\n    endAt: document.getElementById('adjustEnd').value,\n    reason: document.getElementById('adjustReason').value,\n    note: document.getElementById('adjustNote').value.trim(),\n  };\n  if (!payload.employeeCode) {\n    setStatus('请选择员工', document.getElementById('adjustStatus'));\n    return;\n  }\n  if (!payload.startAt || !payload.endAt || !payload.reason || !payload.note) {\n    setStatus('请完整填写补录信息', document.getElementById('adjustStatus'));\n    return;\n  }\n  try {\n    await fetchJSON('/api/v1/admin/manual-adjustments', {\n      method: editingAdjustmentId ? 'PUT' : 'POST',\n      headers: { 'Content-Type': 'application/json' },\n      body: JSON.stringify(payload),\n    });\n    setStatus('保存成功', document.getElementById('adjustStatus'));\n    resetAdjustmentForm();\n    loadAdjustments();\n  } catch (error) {\n    setStatus(error.message, document.getElementById('adjustStatus'));\n  }\n}\n\nasync function revokeAdjustment""",
    "submitAdjustment"
)

sub(
    r"async function loadOfflineSegments\(\)[\s\S]*?function renderOfflineSegments",
    """async function loadOfflineSegments() {\n  const dateInput = document.getElementById('offlineDate');\n  const date = dateInput.value || new Date().toISOString().slice(0, 10);\n  const hidden = document.getElementById('offlineEmployee').value;\n  const typed = document.getElementById('offlineEmployeeSearch').value;\n  const code = resolveEmployeeCode(hidden, typed);\n  if (!code && typed.trim()) {\n    document.getElementById('offlineSegmentsTable').innerHTML = '<div class=\"empty-hint\">请选择员工</div>';\n    return;\n  }\n  let url = '/api/v1/admin/offline-segments?date=' + encodeURIComponent(date);\n  if (code) {\n    url += '&employeeCode=' + encodeURIComponent(code);\n  }\n  try {\n    const items = await fetchJSON(url);\n    renderOfflineSegments(items || []);\n  } catch (error) {\n    document.getElementById('offlineSegmentsTable').innerHTML = '<div class=\"empty-hint\">' + error.message + '</div>';\n  }\n}\n\nfunction renderOfflineSegments""",
    "loadOfflineSegments"
)

sub(
    r"async function loadTimeline\(\)[\s\S]*?function renderTimeline",
    """async function loadTimeline() {\n  const hidden = document.getElementById('timelineEmployee').value;\n  const typed = document.getElementById('timelineEmployeeSearch').value;\n  const code = resolveEmployeeCode(hidden, typed);\n  const dateInput = document.getElementById('timelineDate');\n  const date = dateInput.value || new Date().toISOString().slice(0, 10);\n  if (!code) {\n    updateTimelineHint('请选择员工');\n    document.getElementById('timelineTable').innerHTML = '<div class=\"empty-hint\">请选择员工</div>';\n    return;\n  }\n  try {\n    const data = await fetchJSON('/api/v1/admin/reports/timeline?employeeCode=' + encodeURIComponent(code) + '&date=' + encodeURIComponent(date));\n    renderTimeline(data.items || []);\n  } catch (error) {\n    document.getElementById('timelineTable').innerHTML = '<div class=\"empty-hint\">' + error.message + '</div>';\n  }\n}\n\nfunction renderTimeline""",
    "loadTimeline"
)

sub(
    r"async function initApp\(\)[\s\S]*?document.getElementById\('loginBtn'\)",
    """async function initApp() {\n  if (!authToken) {\n    showLogin();\n    return;\n  }\n  adminNameValue.textContent = adminName || '管理员';\n  timezoneValue.textContent = (Intl.DateTimeFormat().resolvedOptions().timeZone || 'Local');\n  setDefaultDates();\n  await loadSettings();\n  await loadDepartments();\n  await loadEmployees();\n  renderAttendanceStatusOptions();\n  await loadAttendanceRules();\n  await loadCheckoutTemplates();\n  await loadCheckoutTemplatesForFilter();\n  await loadCheckoutRecords(1);\n  await loadReviewList(1);\n  await loadRules();\n  await loadLiveSnapshot();\n  connectLiveWS();\n  startLiveTimer();\n  loadAdjustments();\n  loadIncidents();\n  loadOfflineSegments();\n  loadAuditLogs();\n}\n\ndocument.getElementById('loginBtn')""",
    "initApp"
)

if "async function initApp()" not in content:
    raise SystemExit("initApp insert point not found")
content = content.replace("async function initApp() {", block + "\nasync function initApp() {", 1)

marker_events = "document.getElementById('loadAudit').addEventListener('click', loadAuditLogs);\n\n"
if marker_events in content:
    extra_events = marker_events + """document.getElementById('attendanceDepartment').addEventListener('change', loadAttendanceRules);
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

"""
    content = content.replace(marker_events, extra_events, 1)
else:
    raise SystemExit("event marker not found")

path.write_text(content, encoding="utf-8")
print("app.js updated")
