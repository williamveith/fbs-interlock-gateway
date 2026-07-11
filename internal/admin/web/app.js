let config = null;
let statusRequestInProgress = false;

function escapeHTML(value) {
    return String(value)
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#039;');
}

async function loadConfig() {
    const table = document.getElementById('configTable');

    try {
        const res = await fetch('/api/config', {
            cache: 'no-store'
        });

        if (!res.ok) {
            throw new Error(`HTTP ${res.status}`);
        }

        config = await res.json();
        renderConfig();
    } catch (error) {
        console.error('Failed to load config:', error);

        table.innerHTML = `
            <tr>
                <th>Configuration</th>
            </tr>
            <tr>
                <td class="error">
                    Unable to load configuration:
                    ${escapeHTML(error.message)}
                </td>
            </tr>
        `;
    }
}

async function loadStatus() {
    if (statusRequestInProgress) {
        return;
    }

    statusRequestInProgress = true;

    const table = document.getElementById('statusTable');

    try {
        const res = await fetch('/api/status', {
            cache: 'no-store'
        });

        if (!res.ok) {
            throw new Error(`HTTP ${res.status}`);
        }

        const status = await res.json();
        renderStatus(status);
    } catch (error) {
        console.error('Failed to load live status:', error);

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
            <tr>
                <td colspan="7" class="error">
                    Unable to load live status:
                    ${escapeHTML(error.message)}
                </td>
            </tr>
        `;
    } finally {
        statusRequestInProgress = false;
    }
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
        ${rows.map(row => `
            <tr>
                <td>${escapeHTML(row.interlock_name)}</td>
                <td>${row.port}</td>
                <td>${escapeHTML(row.ip)}</td>
                <td>${row.enabled ? 'yes' : 'no'}</td>
                <td>${row.connected ? 'yes' : 'no'}</td>
                <td class="${row.output ? 'on' : 'off'}">
                    ${row.output ? 'on' : 'off'}
                </td>
                <td class="${row.error ? 'error' : ''}">
                    ${escapeHTML(row.error || '')}
                </td>
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
            <th>Username</th>
            <th>Password</th>
            <th>Delete</th>
        </tr>
        ${config.tools.map((tool, index) => `
            <tr>
                <td>
                    <input
                        type="checkbox"
                        ${tool.enabled ? 'checked' : ''}
                        onchange="config.tools[${index}].enabled = this.checked"
                    >
                </td>
                <td>
                    <input
                        type="text"
                        value="${escapeHTML(tool.interlock_name)}"
                        data-unique="interlock_name"
                        data-index="${index}"
                        required
                        onchange="
                            config.tools[${index}].interlock_name = this.value.trim();
                            validateConfigInputs({ report: true });
                        "
                    >
                </td>
                <td>
                    <input
                        type="text"
                        value="${escapeHTML(tool.ip)}"
                        data-unique="ip"
                        data-index="${index}"
                        required
                        onchange="
                            config.tools[${index}].ip = this.value.trim();
                            validateConfigInputs({ report: true });
                        "
                    >
                </td>
                <td>
                    <input
                        type="number"
                        value="${tool.port}"
                        data-unique="port"
                        data-index="${index}"
                        min="1"
                        max="65535"
                        required
                        onchange="
                            config.tools[${index}].port = Number(this.value);
                            validateConfigInputs({ report: true });
                        "
                    >
                </td>
                <td>
                    <input
                        type="number"
                        value="${tool.switch_id}"
                        onchange="config.tools[${index}].switch_id = Number(this.value)"
                    >
                </td>
                <td>
                    <input
                        type="text"
                        value="${tool.username ?? ''}"
                        onchange="config.tools[${index}].username = this.value"
                    >
                </td>
                <td>
                    <input
                        type="password"
                        value="${tool.password ?? ''}"
                        onchange="config.tools[${index}].password = this.value"
                    >
                </td>
                <td style="text-align: center;">
                    <span class="glyph delete-trigger">&osol;</span>
                </td>
            </tr>
        `).join('')}
    `;
    enableRowDelete()
}

function enableRowDelete() {
    const table = document.getElementById('configTable');

    table.querySelectorAll('.delete-trigger').forEach((trigger, index) => {
        trigger.addEventListener('click', () => {
            config.tools.splice(index, 1);
            renderConfig();
        });
    });
}

function addTool() {
    if (!config) {
        return;
    }

    const nextFreePort = ports => {
        if (ports.length === 0) {
            return 8081;
        }

        const portList = [...ports].sort((a, b) => a - b);

        for (let i = 0; i < portList.length - 1; i++) {
            if (portList[i + 1] - portList[i] > 1) {
                return portList[i] + 1;
            }
        }

        return portList.at(-1) + 1;
    };

    config.tools.push({
        interlock_name: '',
        ip: '',
        port: nextFreePort(config.tools.map(tool => tool.port)),
        switch_id: 0,
        username: null,
        password: null,
        enabled: true
    });

    renderConfig();
}

function normalizeUniqueValue(field, value) {
    const normalized = String(value).trim();

    return field === 'port'
        ? normalized
        : normalized.toLowerCase();
}

function validateConfigInputs({ report = false } = {}) {
    const table = document.getElementById('configTable');
    const fields = ['interlock_name', 'ip', 'port'];
    let firstInvalid = null;

    for (const field of fields) {
        const inputs = [
            ...table.querySelectorAll(`[data-unique="${field}"]`)
        ];

        const counts = new Map();

        for (const input of inputs) {
            const value = normalizeUniqueValue(field, input.value);

            if (value !== '') {
                counts.set(value, (counts.get(value) ?? 0) + 1);
            }
        }

        for (const input of inputs) {
            const value = normalizeUniqueValue(field, input.value);

            let message = '';

            if (value === '') {
                message = 'This field is required.';
            } else if (counts.get(value) > 1) {
                const label = {
                    interlock_name: 'Tool name',
                    ip: 'Shelly host/IP',
                    port: 'Port'
                }[field];

                message = `${label} must be unique.`;
            }

            input.setCustomValidity(message);

            if (!input.checkValidity() && !firstInvalid) {
                firstInvalid = input;
            }
        }
    }

    if (report && firstInvalid) {
        firstInvalid.reportValidity();
        firstInvalid.focus();
    }

    return firstInvalid === null;
}

async function saveConfig() {
    if (!config) {
        alert('Configuration has not loaded.');
        return;
    }

    if (!validateConfigInputs({ report: true })) {
        return;
    }

    try {
        const res = await fetch('/api/config', {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(config)
        });

        if (!res.ok) {
            throw new Error(await res.text());
        }

        const result = await res.json();

        if (!result.restart_required) {
            alert('Config saved.');
            return;
        }

        const restartRes = await fetch('/api/restart', {
            method: 'POST'
        });

        if (!restartRes.ok) {
            throw new Error(await restartRes.text());
        }

        alert(
            'Gateway restarting. Wait a few seconds, then refresh the page.'
        );
    } catch (error) {
        console.error('Failed to save configuration:', error);
        alert(error.message);
    }
}

async function initialize() {
    await Promise.allSettled([
        loadConfig(),
        loadStatus()
    ]);

    setInterval(loadStatus, 3000);
}

initialize();