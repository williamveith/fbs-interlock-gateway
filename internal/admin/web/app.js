'use strict';

// =========================
// CONSTANTS
// =========================

const STATUS_REFRESH_INTERVAL_MS = 3000;
const DEFAULT_STARTING_PORT = 8081;
const MIN_PORT = 8081;
const MAX_PORT = 8981;
const NOTIFICATION_DURATION_MS = 5000;

const STATUS_HEADERS = [
    'Tool',
    'Port',
    'Shelly',
    'Enabled',
    'Connected',
    'Output',
    'Error'
];

const CONFIG_HEADERS = [
    'Enabled',
    'Tool Name',
    'Shelly Host/IP',
    'Port',
    'Switch ID',
    'Username',
    'Password',
    'Delete'
];

// =========================
// APPLICATION STATE
// =========================

let config = null;
let statusRequestInProgress = false;
let configRequestInProgress = false;
let saveRequestInProgress = false;
let statusIntervalID = null;

// =========================
// GENERAL HELPERS
// =========================

function getErrorMessage(error) {
    if (error instanceof Error && error.message) {
        return error.message;
    }

    return String(error);
}

async function readResponseText(response) {
    try {
        return (await response.text()).trim();
    } catch {
        return '';
    }
}

async function readJSONResponse(response, description) {
    if (!response.ok) {
        const responseText = await readResponseText(response);

        throw new Error(
            responseText ||
            `${description} failed with HTTP ${response.status}`
        );
    }

    try {
        return await response.json();
    } catch {
        throw new Error(`${description} returned invalid JSON`);
    }
}

function setElementText(element, value) {
    const text = String(value ?? '');

    if (element.textContent !== text) {
        element.textContent = text;
    }
}

function createCell(text = '') {
    const cell = document.createElement('td');
    cell.textContent = String(text);
    return cell;
}

function createHeaderCell(text) {
    const cell = document.createElement('th');
    cell.textContent = text;
    return cell;
}

function createHeaderRow(headers) {
    const row = document.createElement('tr');

    for (const header of headers) {
        row.appendChild(createHeaderCell(header));
    }

    return row;
}

function replaceTableWithMessage(
    table,
    headers,
    message,
    { error = false } = {}
) {
    table.replaceChildren();

    const thead = document.createElement('thead');
    thead.appendChild(createHeaderRow(headers));

    const tbody = document.createElement('tbody');
    const row = document.createElement('tr');
    const cell = document.createElement('td');

    cell.colSpan = headers.length;
    cell.textContent = message;

    if (error) {
        cell.classList.add('error');
    }

    row.appendChild(cell);
    tbody.appendChild(row);

    table.append(thead, tbody);
}

// =========================
// NOTIFICATIONS
// =========================

function getNotificationElement() {
    let notification = document.getElementById('appNotification');

    if (notification) {
        return notification;
    }

    notification = document.createElement('div');
    notification.id = 'appNotification';
    notification.setAttribute('role', 'status');
    notification.setAttribute('aria-live', 'polite');

    Object.assign(notification.style, {
        position: 'fixed',
        right: '1rem',
        bottom: '1rem',
        zIndex: '1000',
        maxWidth: '32rem',
        padding: '0.75rem 1rem',
        borderRadius: '0.4rem',
        background: '#222',
        color: '#fff',
        boxShadow: '0 0.25rem 1rem rgba(0, 0, 0, 0.25)',
        display: 'none',
        whiteSpace: 'pre-wrap'
    });

    document.body.appendChild(notification);

    return notification;
}

function notify(message, type = 'info', duration = NOTIFICATION_DURATION_MS) {
    const notification = getNotificationElement();

    const backgrounds = {
        info: '#222',
        success: '#146c2e',
        error: '#a61b1b',
        warning: '#8a5a00'
    };

    notification.textContent = String(message);
    notification.style.background = backgrounds[type] ?? backgrounds.info;
    notification.style.display = 'block';

    const existingTimeout = Number(notification.dataset.timeoutId);

    if (existingTimeout) {
        window.clearTimeout(existingTimeout);
    }

    if (duration > 0) {
        const timeoutID = window.setTimeout(() => {
            notification.style.display = 'none';
            delete notification.dataset.timeoutId;
        }, duration);

        notification.dataset.timeoutId = String(timeoutID);
    }
}

// =========================
// BUTTON STATE
// =========================

function setButtonState(button, disabled, busyText = '') {
    if (!button) {
        return;
    }

    if (!button.dataset.originalText) {
        button.dataset.originalText = button.textContent;
    }

    button.disabled = disabled;
    button.textContent = disabled && busyText
        ? busyText
        : button.dataset.originalText;
}

function getAddToolButton() {
    return document.querySelector('button[onclick="addTool()"]');
}

function getSaveConfigButton() {
    return document.querySelector('button[onclick="saveConfig()"]');
}

// =========================
// CONFIGURATION LOADING
// =========================

async function loadConfig() {
    if (configRequestInProgress) {
        return;
    }

    configRequestInProgress = true;

    const table = document.getElementById('configTable');
    const addButton = getAddToolButton();
    const saveButton = getSaveConfigButton();

    setButtonState(addButton, true, 'Loading...');
    setButtonState(saveButton, true, 'Loading...');

    replaceTableWithMessage(
        table,
        CONFIG_HEADERS,
        'Loading configuration...'
    );

    try {
        const response = await fetch('/api/config', {
            method: 'GET',
            cache: 'no-store',
            headers: {
                Accept: 'application/json'
            }
        });

        const loadedConfig = await readJSONResponse(
            response,
            'Loading configuration'
        );

        if (
            !loadedConfig ||
            typeof loadedConfig !== 'object' ||
            !Array.isArray(loadedConfig.tools)
        ) {
            throw new Error('Configuration response has an invalid structure');
        }

        config = loadedConfig;
        renderConfig();
    } catch (error) {
        const message = getErrorMessage(error);

        console.error('Failed to load configuration:', error);

        replaceTableWithMessage(
            table,
            CONFIG_HEADERS,
            `Unable to load configuration: ${message}`,
            { error: true }
        );

        notify(`Unable to load configuration: ${message}`, 'error', 0);
    } finally {
        configRequestInProgress = false;

        setButtonState(addButton, false);
        setButtonState(saveButton, false);
    }
}

// =========================
// STATUS LOADING
// =========================

async function loadStatus() {
    if (statusRequestInProgress || document.hidden) {
        return;
    }

    statusRequestInProgress = true;

    const table = document.getElementById('statusTable');

    try {
        const response = await fetch('/api/status', {
            method: 'GET',
            cache: 'no-store',
            headers: {
                Accept: 'application/json'
            }
        });

        const rows = await readJSONResponse(
            response,
            'Loading live status'
        );

        if (!Array.isArray(rows)) {
            throw new Error('Status response has an invalid structure');
        }

        renderStatus(rows);
    } catch (error) {
        const message = getErrorMessage(error);

        console.error('Failed to load live status:', error);

        renderStatusError(`Unable to load live status: ${message}`);
    } finally {
        statusRequestInProgress = false;
    }
}

// =========================
// STATUS TABLE
// =========================

function initializeStatusTable() {
    const table = document.getElementById('statusTable');

    if (
        table.tHead &&
        table.tHead.rows.length === 1 &&
        table.tBodies.length === 1
    ) {
        return table.tBodies[0];
    }

    table.replaceChildren();

    const thead = document.createElement('thead');
    thead.appendChild(createHeaderRow(STATUS_HEADERS));

    const tbody = document.createElement('tbody');

    table.append(thead, tbody);

    return tbody;
}

function getStatusRowKey(row) {
    // Gateway ports are validated as unique, making the port a stable row key.
    return String(row.port);
}

function createStatusRow(key) {
    const row = document.createElement('tr');
    row.dataset.statusKey = key;

    for (let index = 0; index < STATUS_HEADERS.length; index += 1) {
        row.appendChild(document.createElement('td'));
    }

    return row;
}

function updateStatusRow(tableRow, status) {
    const cells = tableRow.cells;

    const output = Boolean(status.output);
    const errorMessage = String(status.error ?? '');

    setElementText(cells[0], status.interlock_name);
    setElementText(cells[1], status.port);
    setElementText(cells[2], status.ip);
    setElementText(cells[3], status.enabled ? 'yes' : 'no');
    setElementText(cells[4], status.connected ? 'yes' : 'no');
    setElementText(cells[5], output ? 'on' : 'off');
    setElementText(cells[6], errorMessage);

    cells[5].classList.toggle('on', output);
    cells[5].classList.toggle('off', !output);

    cells[6].classList.toggle(
        'error',
        errorMessage.length > 0
    );
}

function renderStatus(rows) {
    const tbody = initializeStatusTable();

    const existingRows = new Map();

    for (const row of tbody.rows) {
        if (row.dataset.statusKey) {
            existingRows.set(row.dataset.statusKey, row);
        }
    }

    let insertionPoint = tbody.firstElementChild;

    for (const status of rows) {
        const key = getStatusRowKey(status);

        let tableRow = existingRows.get(key);

        if (tableRow) {
            existingRows.delete(key);
        } else {
            tableRow = createStatusRow(key);
        }

        updateStatusRow(tableRow, status);

        // Insert new rows and preserve the order returned by the API.
        if (tableRow !== insertionPoint) {
            tbody.insertBefore(tableRow, insertionPoint);
        }

        insertionPoint = tableRow.nextElementSibling;
    }

    for (const staleRow of existingRows.values()) {
        staleRow.remove();
    }

    if (rows.length === 0) {
        const emptyRow = document.createElement('tr');
        emptyRow.dataset.emptyStatusRow = 'true';

        const cell = createCell('No tools configured.');
        cell.colSpan = STATUS_HEADERS.length;

        emptyRow.appendChild(cell);
        tbody.appendChild(emptyRow);
    } else {
        tbody
            .querySelectorAll('[data-empty-status-row="true"]')
            .forEach(row => row.remove());
    }
}

function renderStatusError(message) {
    const tbody = initializeStatusTable();

    tbody.replaceChildren();

    const row = document.createElement('tr');
    const cell = createCell(message);

    cell.colSpan = STATUS_HEADERS.length;
    cell.classList.add('error');

    row.appendChild(cell);
    tbody.appendChild(row);
}

// =========================
// CONFIGURATION TABLE
// =========================

function createInput({
    type = 'text',
    value = '',
    checked = false,
    required = false,
    min = null,
    max = null,
    field,
    unique = false,
    index
}) {
    const input = document.createElement('input');

    input.type = type;
    input.dataset.field = field;
    input.dataset.index = String(index);

    if (type === 'checkbox') {
        input.checked = Boolean(checked);
    } else {
        input.value = value ?? '';
    }

    if (required) {
        input.required = true;
    }

    if (min !== null) {
        input.min = String(min);
    }

    if (max !== null) {
        input.max = String(max);
    }

    if (unique) {
        input.dataset.unique = field;
    }

    return input;
}

function createConfigRow(tool, index) {
    const row = document.createElement('tr');
    row.dataset.index = String(index);

    const enabledCell = document.createElement('td');
    enabledCell.appendChild(createInput({
        type: 'checkbox',
        checked: tool.enabled,
        field: 'enabled',
        index
    }));

    const nameCell = document.createElement('td');
    nameCell.appendChild(createInput({
        value: tool.interlock_name,
        field: 'interlock_name',
        unique: true,
        required: true,
        index
    }));

    const ipCell = document.createElement('td');
    ipCell.appendChild(createInput({
        value: tool.ip,
        field: 'ip',
        unique: true,
        required: true,
        index
    }));

    const portCell = document.createElement('td');
    portCell.appendChild(createInput({
        type: 'number',
        value: tool.port,
        field: 'port',
        unique: true,
        required: true,
        min: MIN_PORT,
        max: MAX_PORT,
        index
    }));

    const switchIDCell = document.createElement('td');
    switchIDCell.appendChild(createInput({
        type: 'number',
        value: tool.switch_id,
        field: 'switch_id',
        index
    }));

    const usernameCell = document.createElement('td');
    usernameCell.appendChild(createInput({
        value: tool.username ?? '',
        field: 'username',
        index
    }));

    const passwordCell = document.createElement('td');
    const passwordInput = createInput({
        type: 'password',
        value: '',
        field: 'password',
        index
    });
    passwordInput.placeholder = tool.password_set
        ? 'Password Set'
        : 'No Password Set';
    passwordInput.autocomplete = 'new-password';
    passwordCell.appendChild(passwordInput);

    const deleteCell = document.createElement('td');
    deleteCell.style.textAlign = 'center';

    const deleteTrigger = document.createElement('span');
    deleteTrigger.className = 'glyph delete-trigger';
    deleteTrigger.innerHTML = '&osol;';

    deleteCell.appendChild(deleteTrigger);

    row.append(
        enabledCell,
        nameCell,
        ipCell,
        portCell,
        switchIDCell,
        usernameCell,
        passwordCell,
        deleteCell
    );

    return row;
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

function renderConfig() {
    const table = document.getElementById('configTable');

    table.replaceChildren();

    const thead = document.createElement('thead');
    thead.appendChild(createHeaderRow(CONFIG_HEADERS));

    const tbody = document.createElement('tbody');

    for (let index = 0; index < config.tools.length; index += 1) {
        tbody.appendChild(createConfigRow(config.tools[index], index));
    }

    if (config.tools.length === 0) {
        const row = document.createElement('tr');
        const cell = createCell('No tools configured.');

        cell.colSpan = CONFIG_HEADERS.length;

        row.appendChild(cell);
        tbody.appendChild(row);
    }

    table.append(thead, tbody);

    enableRowDelete();
    validateConfigInputs();
}

// =========================
// CONFIGURATION EDITING
// =========================

function parseInputValue(input) {
    switch (input.dataset.field) {
    case 'enabled':
        return input.checked;

    case 'port':
    case 'switch_id':
        return input.value === ''
            ? 0
            : Number(input.value);

    case 'interlock_name':
    case 'ip':
        return input.value.trim();

    case 'username':
        return input.value === ''
            ? null
            : input.value;

    case 'password':
        return input.value === ''
            ? null
            : input.value;

    default:
        return input.value;
    }
}

function handleConfigInput(event) {
    const input = event.target.closest('input[data-field]');

    if (!input || !config) {
        return;
    }

    const index = Number(input.dataset.index);
    const field = input.dataset.field;
    const tool = config.tools[index];

    if (!tool || !field) {
        return;
    }

    tool[field] = parseInputValue(input);

    if (field === 'password') {
        tool.password_set = input.value !== '';
        tool.clear_password = false;
    }

    if (input.dataset.unique) {
        validateConfigInputs({
            report: event.type === 'change'
        });
    }
}

function handleConfigClick(event) {
    const deleteButton = event.target.closest('.delete-trigger');

    if (!deleteButton || !config) {
        return;
    }

    const index = Number(deleteButton.dataset.index);

    if (!Number.isInteger(index) || !config.tools[index]) {
        return;
    }

    config.tools.splice(index, 1);
    renderConfig();
}

function enableConfigEventDelegation() {
    const table = document.getElementById('configTable');

    table.addEventListener('input', handleConfigInput);
    table.addEventListener('change', handleConfigInput);
    table.addEventListener('click', handleConfigClick);
}

// =========================
// ADDING TOOLS
// =========================

function getNextFreePort(ports) {
    const validPorts = ports
        .map(Number)
        .filter(port =>
            Number.isInteger(port) &&
            port >= MIN_PORT &&
            port <= MAX_PORT
        );

    if (validPorts.length === 0) {
        return DEFAULT_STARTING_PORT;
    }

    const uniquePorts = [...new Set(validPorts)]
        .sort((a, b) => a - b);

    let candidate = DEFAULT_STARTING_PORT;

    for (const port of uniquePorts) {
        if (port < candidate) {
            continue;
        }

        if (port === candidate) {
            candidate += 1;
            continue;
        }

        if (port > candidate) {
            break;
        }
    }

    if (candidate > MAX_PORT) {
        throw new Error('No free TCP port is available');
    }

    return candidate;
}

function addTool() {
    if (!config || saveRequestInProgress) {
        notify('Configuration has not finished loading.', 'warning');
        return;
    }

    try {
        const nextPort = getNextFreePort(
            config.tools.map(tool => tool.port)
        );

        config.tools.push({
            interlock_name: '',
            ip: '',
            port: nextPort,
            switch_id: 0,
            username: null,
            password: null,
            password_set: false,
            clear_password: false,
            enabled: true
        });

        renderConfig();

        const newIndex = config.tools.length - 1;
        const newInput = document.querySelector(
            `#configTable input[data-field="interlock_name"]` +
            `[data-index="${newIndex}"]`
        );

        newInput?.focus();
    } catch (error) {
        const message = getErrorMessage(error);

        console.error('Failed to add tool:', error);
        notify(message, 'error');
    }
}

// =========================
// VALIDATION
// =========================

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
            ...table.querySelectorAll(
                `[data-unique="${field}"]`
            )
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
            } else if (
                field === 'port' &&
                (
                    !Number.isInteger(Number(value)) ||
                    Number(value) < MIN_PORT ||
                    Number(value) > MAX_PORT
                )
            ) {
                message = `Port must be between ${MIN_PORT} and ${MAX_PORT}.`;
            } else if ((counts.get(value) ?? 0) > 1) {
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
        firstInvalid.focus();
        firstInvalid.reportValidity();
    }

    return firstInvalid === null;
}

// =========================
// SAVING CONFIGURATION
// =========================

async function saveConfig() {
    if (!config) {
        notify('Configuration has not loaded.', 'warning');
        return;
    }

    if (saveRequestInProgress) {
        return;
    }

    if (!validateConfigInputs({ report: true })) {
        notify(
            'Correct the highlighted configuration fields before saving.',
            'warning'
        );
        return;
    }

    saveRequestInProgress = true;

    const addButton = getAddToolButton();
    const saveButton = getSaveConfigButton();

    setButtonState(addButton, true);
    setButtonState(saveButton, true, 'Saving...');

    try {
        const response = await fetch('/api/config', {
            method: 'PUT',
            cache: 'no-store',
            headers: {
                'Content-Type': 'application/json',
                Accept: 'application/json'
            },
            body: JSON.stringify(config)
        });

        const result = await readJSONResponse(
            response,
            'Saving configuration'
        );

        if (!result.restart_required) {
            notify('Configuration saved.', 'success');
            return;
        }

        notify(
            'Configuration saved. Restarting gateway...',
            'info',
            0
        );

        const restartResponse = await fetch('/api/restart', {
            method: 'POST',
            cache: 'no-store',
            headers: {
                Accept: 'application/json'
            }
        });

        await readJSONResponse(
            restartResponse,
            'Restarting gateway'
        );

        notify(
            'Gateway is restarting. Wait a few seconds, then refresh the page.',
            'success',
            0
        );
    } catch (error) {
        const message = getErrorMessage(error);

        console.error('Failed to save configuration:', error);
        notify(`Failed to save configuration: ${message}`, 'error', 0);
    } finally {
        saveRequestInProgress = false;

        setButtonState(addButton, false);
        setButtonState(saveButton, false);
    }
}

// =========================
// STATUS POLLING
// =========================

function startStatusPolling() {
    if (statusIntervalID !== null) {
        window.clearInterval(statusIntervalID);
    }

    statusIntervalID = window.setInterval(() => {
        void loadStatus();
    }, STATUS_REFRESH_INTERVAL_MS);
}

function handleVisibilityChange() {
    if (!document.hidden) {
        void loadStatus();
    }
}

// =========================
// INITIALIZATION
// =========================

async function initialize() {
    enableConfigEventDelegation();

    document.addEventListener(
        'visibilitychange',
        handleVisibilityChange
    );

    await Promise.allSettled([
        loadConfig(),
        loadStatus()
    ]);

    startStatusPolling();
}

void initialize();