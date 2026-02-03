$path = "internal/web/static/assets/app.js"
$content = Get-Content -Raw -Path $path

$content = $content -replace 'let editingEmployeeId = null;\r?\n', "let editingEmployeeId = null;`r`nlet editingCheckoutTemplateId = null;`r`nlet editingCheckoutFieldId = null;`r`n`r`n"
$content = $content -replace 'let employees = \[\];\r?\n', "let employees = [];`r`nlet checkoutTemplates = [];`r`nlet checkoutFields = [];`r`n"

$content = $content -replace 'const employeeSelect = document.getElementById\(''employeeDepartment''\);', "const employeeSelect = document.getElementById('employeeDepartment');`r`n  const checkoutSelect = document.getElementById('checkoutDepartment');"
$content = $content -replace 'if \(employeeSelect\) \{\r?\n\s+employeeSelect.innerHTML = ''<option value=\"0\">未分配</option>'' \+ options;\r?\n\s+\}', "if (employeeSelect) {`r`n    employeeSelect.innerHTML = '<option value=\"0\">未分配</option>' + options;`r`n  }`r`n  if (checkoutSelect) {`r`n    checkoutSelect.innerHTML = '<option value=\"0\">请选择部门</option>' + options;`r`n  }"

$checkoutBlock = @"
function formatCheckoutFieldType(type) {
  if (type === 'text') return '文本';
  if (type === 'number') return '数字';
  if (type === 'select') return '下拉';
  return type || '-';
}

function resetCheckoutTemplateForm() {
  editingCheckoutTemplateId = null;
  const nameInput = document.getElementById('checkoutTemplateName');
  const enabledInput = document.getElementById('checkoutTemplateEnabled');
  const saveBtn = document.getElementById('saveCheckoutTemplate');
  if (nameInput) nameInput.value = '';
  if (enabledInput) enabledInput.checked = false;
  if (saveBtn) saveBtn.textContent = '保存模板';
}

function fillCheckoutTemplateForm(item) {
  editingCheckoutTemplateId = item.id;
  const nameInput = document.getElementById('checkoutTemplateName');
  const enabledInput = document.getElementById('checkoutTemplateEnabled');
  const saveBtn = document.getElementById('saveCheckoutTemplate');
  if (nameInput) nameInput.value = item.name || '';
  if (enabledInput) enabledInput.checked = !!item.enabled;
  if (saveBtn) saveBtn.textContent = '更新模板';
  loadCheckoutFields(item.id);
}

async function loadCheckoutTemplates(selectedId) {
  const deptSelect = document.getElementById('checkoutDepartment');
  const deptId = Number(deptSelect ? deptSelect.value : 0);
  if (!deptId) {
    checkoutTemplates = [];
    renderCheckoutTemplates([]);
    resetCheckoutTemplateForm();
    resetCheckoutFieldForm();
    renderCheckoutFields([]);
    return;
  }
  try {
    const items = await fetchJSON('/api/v1/admin/checkout-templates?departmentId=' + deptId);
    checkoutTemplates = Array.isArray(items) ? items : [];
    renderCheckoutTemplates(checkoutTemplates);
    const targetId = selectedId || editingCheckoutTemplateId;
    if (targetId) {
      const target = checkoutTemplates.find((row) => row.id === targetId);
      if (target) {
        fillCheckoutTemplateForm(target);
        return;
      }
    }
    if (checkoutTemplates.length === 1) {
      fillCheckoutTemplateForm(checkoutTemplates[0]);
    } else {
      resetCheckoutTemplateForm();
      resetCheckoutFieldForm();
      renderCheckoutFields([]);
    }
  } catch (error) {
    document.getElementById('checkoutTemplatesTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
    renderCheckoutFields([]);
  }
}

async function saveCheckoutTemplate() {
  const statusEl = document.getElementById('checkoutTemplateStatus');
  const deptId = Number(document.getElementById('checkoutDepartment').value || 0);
  const name = document.getElementById('checkoutTemplateName').value.trim();
  const enabled = document.getElementById('checkoutTemplateEnabled').checked;
  if (!deptId) {
    setStatus('请选择部门', statusEl);
    return;
  }
  if (!name) {
    setStatus('请输入模板名称', statusEl);
    return;
  }
  const payload = {
    id: editingCheckoutTemplateId || 0,
    departmentId: deptId,
    name: name,
    enabled: enabled,
  };
  try {
    const data = await fetchJSON('/api/v1/admin/checkout-templates', {
      method: editingCheckoutTemplateId ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', statusEl);
    const targetId = data && data.id ? data.id : editingCheckoutTemplateId;
    await loadCheckoutTemplates(targetId);
  } catch (error) {
    setStatus(error.message, statusEl);
  }
}

async function toggleCheckoutTemplate(id) {
  const item = checkoutTemplates.find((row) => row.id === id);
  if (!item) return;
  try {
    await fetchJSON('/api/v1/admin/checkout-templates', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: item.id, name: item.name, enabled: !item.enabled }),
    });
    await loadCheckoutTemplates(item.id);
  } catch (error) {
    setStatus(error.message, document.getElementById('checkoutTemplateStatus'));
  }
}

async function deleteCheckoutTemplate(id) {
  try {
    await fetchJSON('/api/v1/admin/checkout-templates?id=' + id, { method: 'DELETE' });
    if (editingCheckoutTemplateId === id) {
      resetCheckoutTemplateForm();
      resetCheckoutFieldForm();
      renderCheckoutFields([]);
    }
    await loadCheckoutTemplates();
  } catch (error) {
    setStatus(error.message, document.getElementById('checkoutTemplateStatus'));
  }
}

function renderCheckoutTemplates(items) {
  const container = document.getElementById('checkoutTemplatesTable');
  const headers = ['模板名称', '状态', '字段数', '更新时间', '操作'];
  const rows = items.map((item) => {
    const statusText = item.enabled ? '启用' : '停用';
    const toggleText = item.enabled ? '停用' : '启用';
    return [
      '<div class="table-row cols-5">',
      '<div>' + item.name + '</div>',
      '<div>' + statusText + '</div>',
      '<div>' + item.fieldCount + '</div>',
      '<div>' + item.updatedAt + '</div>',
      '<div>',
      '<button class="btn btn-secondary" data-action="edit" data-id="' + item.id + '">编辑</button>',
      '<button class="btn btn-secondary" data-action="toggle" data-id="' + item.id + '">' + toggleText + '</button>',
      '<button class="btn btn-secondary" data-action="delete" data-id="' + item.id + '">删除</button>',
      '</div>',
      '</div>'
    ].join('');
  });
  renderTable(container, headers, rows, 'cols-5');
}

function setCheckoutFieldOptionsState() {
  const typeSelect = document.getElementById('checkoutFieldType');
  const optionsInput = document.getElementById('checkoutFieldOptions');
  if (!typeSelect || !optionsInput) return;
  if (typeSelect.value === 'select') {
    optionsInput.disabled = false;
  } else {
    optionsInput.disabled = true;
    optionsInput.value = '';
  }
}

function resetCheckoutFieldForm() {
  editingCheckoutFieldId = null;
  const nameInput = document.getElementById('checkoutFieldName');
  const typeSelect = document.getElementById('checkoutFieldType');
  const requiredInput = document.getElementById('checkoutFieldRequired');
  const enabledInput = document.getElementById('checkoutFieldEnabled');
  const sortInput = document.getElementById('checkoutFieldSort');
  const optionsInput = document.getElementById('checkoutFieldOptions');
  const saveBtn = document.getElementById('saveCheckoutField');
  if (nameInput) nameInput.value = '';
  if (typeSelect) typeSelect.value = 'text';
  if (requiredInput) requiredInput.checked = false;
  if (enabledInput) enabledInput.checked = true;
  if (sortInput) sortInput.value = '0';
  if (optionsInput) optionsInput.value = '';
  if (saveBtn) saveBtn.textContent = '保存字段';
  setCheckoutFieldOptionsState();
}

function fillCheckoutFieldForm(item) {
  editingCheckoutFieldId = item.id;
  const nameInput = document.getElementById('checkoutFieldName');
  const typeSelect = document.getElementById('checkoutFieldType');
  const requiredInput = document.getElementById('checkoutFieldRequired');
  const enabledInput = document.getElementById('checkoutFieldEnabled');
  const sortInput = document.getElementById('checkoutFieldSort');
  const optionsInput = document.getElementById('checkoutFieldOptions');
  const saveBtn = document.getElementById('saveCheckoutField');
  if (nameInput) nameInput.value = item.name || '';
  if (typeSelect) typeSelect.value = item.type || 'text';
  if (requiredInput) requiredInput.checked = !!item.required;
  if (enabledInput) enabledInput.checked = !!item.enabled;
  if (sortInput) sortInput.value = String(item.sortOrder || 0);
  if (optionsInput) optionsInput.value = (item.options || []).join('\n');
  if (saveBtn) saveBtn.textContent = '更新字段';
  setCheckoutFieldOptionsState();
}

async function loadCheckoutFields(templateId) {
  const currentId = templateId || editingCheckoutTemplateId;
  if (!currentId) {
    renderCheckoutFields([]);
    return;
  }
  try {
    const items = await fetchJSON('/api/v1/admin/checkout-fields?templateId=' + currentId);
    checkoutFields = Array.isArray(items) ? items : [];
    renderCheckoutFields(checkoutFields);
  } catch (error) {
    document.getElementById('checkoutFieldsTable').innerHTML = '<div class="empty-hint">' + error.message + '</div>';
  }
}

function renderCheckoutFields(items) {
  const container = document.getElementById('checkoutFieldsTable');
  const headers = ['字段名称', '类型', '必填', '选项', '排序', '状态', '操作'];
  const rows = items.map((item) => {
    const requiredText = item.required ? '必填' : '选填';
    const statusText = item.enabled ? '启用' : '停用';
    const optionsText = item.type === 'select' ? (item.options || []).join(' / ') : '-';
    return [
      '<div class="table-row cols-7">',
      '<div>' + item.name + '</div>',
      '<div>' + formatCheckoutFieldType(item.type) + '</div>',
      '<div>' + requiredText + '</div>',
      '<div>' + (optionsText || '-') + '</div>',
      '<div>' + (item.sortOrder || 0) + '</div>',
      '<div>' + statusText + '</div>',
      '<div>',
      '<button class="btn btn-secondary" data-action="edit" data-id="' + item.id + '">编辑</button>',
      '<button class="btn btn-secondary" data-action="delete" data-id="' + item.id + '">删除</button>',
      '</div>',
      '</div>'
    ].join('');
  });
  renderTable(container, headers, rows, 'cols-7');
}

function parseCheckoutFieldOptions() {
  const input = document.getElementById('checkoutFieldOptions');
  if (!input) return [];
  const lines = input.value.split(/\r?\n/);
  const options = [];
  lines.forEach((line) => {
    line.split(/,|，/).forEach((part) => {
      const value = part.trim();
      if (value) {
        options.push(value);
      }
    });
  });
  return options;
}

async function saveCheckoutField() {
  const statusEl = document.getElementById('checkoutFieldStatus');
  const templateId = editingCheckoutTemplateId;
  if (!templateId) {
    setStatus('请先选择模板', statusEl);
    return;
  }
  const name = document.getElementById('checkoutFieldName').value.trim();
  const type = document.getElementById('checkoutFieldType').value;
  const required = document.getElementById('checkoutFieldRequired').checked;
  const enabled = document.getElementById('checkoutFieldEnabled').checked;
  const sortOrder = Number(document.getElementById('checkoutFieldSort').value || 0);
  const options = parseCheckoutFieldOptions();
  if (!name) {
    setStatus('请输入字段名称', statusEl);
    return;
  }
  if (type === 'select' && options.length === 0) {
    setStatus('请填写下拉选项', statusEl);
    return;
  }
  const payload = {
    id: editingCheckoutFieldId || 0,
    templateId: templateId,
    name: name,
    type: type,
    required: required,
    enabled: enabled,
    sortOrder: sortOrder,
    options: options,
  };
  try {
    await fetchJSON('/api/v1/admin/checkout-fields', {
      method: editingCheckoutFieldId ? 'PUT' : 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    setStatus('保存成功', statusEl);
    resetCheckoutFieldForm();
    await loadCheckoutFields(templateId);
  } catch (error) {
    setStatus(error.message, statusEl);
  }
}

async function deleteCheckoutField(id) {
  try {
    await fetchJSON('/api/v1/admin/checkout-fields?id=' + id, { method: 'DELETE' });
    await loadCheckoutFields(editingCheckoutTemplateId);
  } catch (error) {
    setStatus(error.message, document.getElementById('checkoutFieldStatus'));
  }
}
"@

$content = $content -replace 'async function initApp\(\) \{', ($checkoutBlock + "`r`nasync function initApp() {")

$eventBlock = @"
const checkoutDepartment = document.getElementById('checkoutDepartment');
if (checkoutDepartment) {
  checkoutDepartment.addEventListener('change', () => {
    resetCheckoutTemplateForm();
    resetCheckoutFieldForm();
    loadCheckoutTemplates();
  });
}

const checkoutFieldType = document.getElementById('checkoutFieldType');
if (checkoutFieldType) {
  checkoutFieldType.addEventListener('change', setCheckoutFieldOptionsState);
}

const saveCheckoutTemplateBtn = document.getElementById('saveCheckoutTemplate');
if (saveCheckoutTemplateBtn) {
  saveCheckoutTemplateBtn.addEventListener('click', saveCheckoutTemplate);
}
const cancelCheckoutTemplateBtn = document.getElementById('cancelCheckoutTemplate');
if (cancelCheckoutTemplateBtn) {
  cancelCheckoutTemplateBtn.addEventListener('click', resetCheckoutTemplateForm);
}

const saveCheckoutFieldBtn = document.getElementById('saveCheckoutField');
if (saveCheckoutFieldBtn) {
  saveCheckoutFieldBtn.addEventListener('click', saveCheckoutField);
}
const cancelCheckoutFieldBtn = document.getElementById('cancelCheckoutField');
if (cancelCheckoutFieldBtn) {
  cancelCheckoutFieldBtn.addEventListener('click', resetCheckoutFieldForm);
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
      if (item) {
        fillCheckoutTemplateForm(item);
      }
    }
    if (action === 'toggle') {
      toggleCheckoutTemplate(id);
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
      if (item) {
        fillCheckoutFieldForm(item);
      }
    }
    if (action === 'delete') {
      deleteCheckoutField(id);
    }
  });
}
"@

$content = $content -replace 'document.getElementById\(''cancelEmployee''\)\.addEventListener\(''click'', resetEmployeeForm\);', "document.getElementById('cancelEmployee').addEventListener('click', resetEmployeeForm);`r`n`r`n$eventBlock"

$content = $content -replace 'await loadDepartments\(\);', "await loadDepartments();`r`n  await loadCheckoutTemplates();"

Set-Content -Path $path -Value $content -Encoding utf8
