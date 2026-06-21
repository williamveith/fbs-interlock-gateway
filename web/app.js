let config = null;

async function loadConfig() {
    const res = await fetch('/api/config');
    config = await res.json();
    renderConfig();
}

async function loadStatus() {
    const res = await fetch('/api/status');
    const status = await res.json();
    renderStatus(status);
}

function renderStatus(rows) {
    const table = document.getElementById('statusTable');

    table.innerHTML = `
    <tr>
      <th>Tool</th>
      <th>Port</th>
      <th>Shelly</th>
      <th>Enabled</th>
      <th>Connected</th>
      <th>Output</th>
      <th>Error</th>
    </tr>
    ${rows.map(r => `
      <tr>
        <td>${r.interlock_name}</td>
        <td>${r.port}</td>
        <td>${r.ip}</td>
        <td>${r.enabled ? 'yes' : 'no'}</td>
        <td>${r.connected ? 'yes' : 'no'}</td>
        <td class="${r.output ? 'on' : ''}">${r.output ? 'on' : 'off'}</td>
        <td class="error">${r.error || ''}</td>
      </tr>
    `).join('')}
  `;
}

function renderConfig() {
    const table = document.getElementById('configTable');

    table.innerHTML = `
    <tr>
      <th>Enabled</th>
      <th>Tool Name</th>
      <th>Shelly Host/IP</th>
      <th>Port</th>
      <th>Switch ID</th>
    </tr>
    ${config.tools.map((t, i) => `
      <tr>
        <td><input type="checkbox" ${t.enabled ? 'checked' : ''} onchange="config.tools[${i}].enabled = this.checked"></td>
        <td><input value="${t.interlock_name}" onchange="config.tools[${i}].interlock_name = this.value"></td>
        <td><input value="${t.ip}" onchange="config.tools[${i}].ip = this.value"></td>
        <td><input type="number" value="${t.port}" onchange="config.tools[${i}].port = Number(this.value)"></td>
        <td><input type="number" value="${t.switch_id}" onchange="config.tools[${i}].switch_id = Number(this.value)"></td>
      </tr>
    `).join('')}
  `;
}

function addTool() {
    config.tools.push({
        interlock_name: '',
        ip: '',
        port: 8080,
        switch_id: 0,
        username: null,
        password: null,
        enabled: true
    });

    renderConfig();
}

async function saveConfig() {
    const res = await fetch('/api/config', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config)
    });

    if (!res.ok) {
        alert(await res.text());
        return;
    }

    const result = await res.json();

    if (result.restart_required) {
        const restartRes = await fetch('/api/restart', {
            method: 'POST'
        });

        if (!restartRes.ok) {
            alert(await restartRes.text());
            return;
        }

        alert('Gateway restarting. Wait a few seconds, then refresh the page.');
    } else {
        alert('Config saved.');
    }
}

loadConfig();
loadStatus();
setInterval(loadStatus, 3000);