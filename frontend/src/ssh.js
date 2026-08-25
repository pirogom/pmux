import { state } from './state.js';
import { dom, addClick } from './dom.js';
import { apiGet, apiPost } from './api.js';
import { showToast, showConfirm } from './ui.js';

const DEFAULT_SSH_CLIENT_PATH = 'C:\\Windows\\System32\\OpenSSH\\ssh.exe';

let sshConfig = null;
let sshPaneId = null;
let editingSshAddressId = null;
let sshPassMode = 'export'; // 'export' | 'import'
let sshPassResolve = null;

// Load the (global) ssh config. Wails binding first, daemon HTTP API as fallback.
async function loadSSHConfig() {
    if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetSSHConfig) {
        return await window.go.main.App.GetSSHConfig();
    }
    try {
        return await apiGet('/api/ssh/config');
    } catch (e) {
        return null;
    }
}

// Persist the (global) ssh config. Returns true on success.
async function persistSSHConfig() {
    if (!sshConfig) return false;
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.SaveSSHConfig) {
            await window.go.main.App.SaveSSHConfig(sshConfig);
        } else {
            await apiPost('/api/ssh/config', sshConfig);
        }
        return true;
    } catch (e) {
        showToast('Failed to save SSH config: ' + (e.message || e), 'error');
        return false;
    }
}

// Save the ssh client path from the manager input immediately.
async function saveSSHClientPath() {
    if (!sshConfig || !dom.sshClientPathInput) return;
    sshConfig.clientPath = dom.sshClientPathInput.value.trim() || DEFAULT_SSH_CLIENT_PATH;
    const saved = await persistSSHConfig();
    if (saved) {
        showToast('SSH client path saved', 'success');
    }
}

export async function openSSHManager(sessionId, paneId) {
    sshPaneId = paneId;
    try {
        const loaded = await loadSSHConfig();
        if (loaded) {
            sshConfig = loaded;
        } else {
            sshConfig = { clientPath: DEFAULT_SSH_CLIENT_PATH, addresses: [] };
        }
    } catch (e) {
        const msg = (e && e.message) || String(e);
        if (/version/i.test(msg)) {
            showToast('SSH config version is incompatible. Config was not loaded.', 'error');
        } else {
            showToast('Failed to load SSH config: ' + msg, 'error');
        }
        sshConfig = { clientPath: DEFAULT_SSH_CLIENT_PATH, addresses: [] };
    }
    if (dom.sshClientPathInput) {
        dom.sshClientPathInput.value = (sshConfig && sshConfig.clientPath) || DEFAULT_SSH_CLIENT_PATH;
    }
    renderSSHAddressList();
    dom.sshModalEl.classList.remove('hidden');
}

function closeSSHManager() {
    dom.sshModalEl.classList.add('hidden');
    sshPaneId = null;
}

function renderSSHAddressList() {
    dom.sshAddressListEl.innerHTML = '';
    const list = (sshConfig && sshConfig.addresses) || [];
    if (list.length === 0) {
        const empty = document.createElement('div');
        empty.className = 'ssh-empty';
        empty.textContent = 'No addresses saved yet. Click "Add Address" to create one.';
        dom.sshAddressListEl.appendChild(empty);
        return;
    }

    list.forEach(addr => {
        const item = document.createElement('div');
        item.className = 'ssh-address-item';

        const info = document.createElement('div');
        info.className = 'ssh-address-item-info';
        const name = document.createElement('div');
        name.className = 'ssh-address-item-name';
        name.textContent = addr.name || '(unnamed)';
        const desc = document.createElement('div');
        desc.className = 'ssh-address-item-desc';
        desc.textContent = addr.user ? `${addr.user}@${addr.host}` : addr.host;
        if (addr.description) {
            desc.textContent += ` — ${addr.description}`;
        }
        info.appendChild(name);
        info.appendChild(desc);

        const actions = document.createElement('div');
        actions.className = 'ssh-address-item-actions';

        const connectBtn = document.createElement('button');
        connectBtn.className = 'btn btn-primary btn-sm';
        connectBtn.textContent = 'Connect';
        connectBtn.title = 'Connect to this address in the pane terminal';
        connectBtn.addEventListener('click', () => sshConnect(addr));

        const editBtn = document.createElement('button');
        editBtn.className = 'icon-btn-small';
        editBtn.textContent = '✏️';
        editBtn.title = 'Edit Address';
        editBtn.addEventListener('click', () => openSSHAddressModal(addr));

        const delBtn = document.createElement('button');
        delBtn.className = 'icon-btn-small del-btn';
        delBtn.textContent = '✖';
        delBtn.title = 'Delete Address';
        delBtn.addEventListener('click', async () => {
            const confirmed = await showConfirm('Delete Address', `Are you sure you want to delete "${addr.name}"?`);
            if (!confirmed) return;
            sshConfig.addresses = sshConfig.addresses.filter(a => a.id !== addr.id);
            renderSSHAddressList();
            await persistSSHConfig();
            showToast('Address deleted', 'info');
        });

        actions.appendChild(connectBtn);
        actions.appendChild(editBtn);
        actions.appendChild(delBtn);

        item.appendChild(info);
        item.appendChild(actions);
        dom.sshAddressListEl.appendChild(item);
    });
}

function openSSHAddressModal(addrToEdit) {
    editingSshAddressId = addrToEdit ? addrToEdit.id : null;
    const titleEl = document.getElementById('ssh-address-modal-title');
    if (titleEl) titleEl.textContent = addrToEdit ? 'Edit Address' : 'Add Address';
    dom.sshAddrNameInput.value = addrToEdit ? (addrToEdit.name || '') : '';
    dom.sshAddrDescInput.value = addrToEdit ? (addrToEdit.description || '') : '';
    dom.sshAddrHostInput.value = addrToEdit ? (addrToEdit.host || '') : '';
    dom.sshAddrUserInput.value = addrToEdit ? (addrToEdit.user || '') : '';
    dom.sshAddressModalEl.classList.remove('hidden');
    setTimeout(() => dom.sshAddrNameInput.focus(), 50);
}

function closeSSHAddressModal() {
    editingSshAddressId = null;
    dom.sshAddressModalEl.classList.add('hidden');
}

async function saveSSHAddressFromModal() {
    const name = dom.sshAddrNameInput.value.trim();
    const host = dom.sshAddrHostInput.value.trim();
    if (!name || !host) {
        showToast('Please specify name and host address', 'error');
        return;
    }

    const addr = {
        id: editingSshAddressId || `ssh_addr_${Date.now()}`,
        name,
        description: dom.sshAddrDescInput.value.trim(),
        host,
        user: dom.sshAddrUserInput.value.trim()
    };

    if (!sshConfig) sshConfig = { clientPath: DEFAULT_SSH_CLIENT_PATH, addresses: [] };
    if (!sshConfig.addresses) sshConfig.addresses = [];

    if (editingSshAddressId) {
        const idx = sshConfig.addresses.findIndex(a => a.id === editingSshAddressId);
        if (idx !== -1) {
            sshConfig.addresses[idx] = addr;
        }
    } else {
        sshConfig.addresses.push(addr);
    }

    renderSSHAddressList();
    closeSSHAddressModal();
    const saved = await persistSSHConfig();
    showToast(saved ? 'Address saved' : 'Address saved, but failed to persist', saved ? 'success' : 'error');
}

function sshConnect(addr) {
    if (!sshPaneId) {
        showToast('No pane selected for SSH connection', 'error');
        return;
    }
    const paneEntry = state.activePanesMap.get(sshPaneId);
    if (!paneEntry || !paneEntry.ws || paneEntry.ws.readyState !== WebSocket.OPEN) {
        showToast('Terminal connection is not available for this pane', 'error');
        return;
    }

    const target = addr.user ? `${addr.user}@${addr.host}` : addr.host;
    if (!target) {
        showToast('Address has no host', 'error');
        return;
    }

    const clientPath = (sshConfig && sshConfig.clientPath) || DEFAULT_SSH_CLIENT_PATH;
    let cmd;
    if (!clientPath || clientPath.trim().toLowerCase() === DEFAULT_SSH_CLIENT_PATH.toLowerCase()) {
        // System OpenSSH lives in System32, which is always on PATH
        cmd = `ssh ${target}`;
    } else {
        const shell = (paneEntry.command || '').toLowerCase();
        const isPowershell = shell.includes('powershell') || shell.includes('pwsh');
        const quoted = `"${clientPath.trim()}"`;
        cmd = isPowershell ? `& ${quoted} ${target}` : `${quoted} ${target}`;
    }

    paneEntry.ws.send(JSON.stringify({ type: 'input', data: cmd + '\r' }));
    showToast(`SSH connection started: ${cmd}`, 'success');
    closeSSHManager();
}

function openSSHPasswordModal(mode, title, message) {
    return new Promise((resolve) => {
        sshPassMode = mode;
        sshPassResolve = resolve;
        dom.sshPassTitleEl.textContent = title;
        dom.sshPassMessageEl.textContent = message;
        dom.sshPassInput.value = '';
        dom.sshPassConfirmInput.value = '';
        dom.sshPassConfirmGroupEl.classList.toggle('hidden', mode !== 'export');
        dom.sshPassModalEl.classList.remove('hidden');
        setTimeout(() => dom.sshPassInput.focus(), 50);
    });
}

function cancelSSHPasswordModal() {
    dom.sshPassModalEl.classList.add('hidden');
    if (sshPassResolve) {
        sshPassResolve(null);
        sshPassResolve = null;
    }
}

function confirmSSHPasswordModal() {
    const pwd = dom.sshPassInput.value;
    if (sshPassMode === 'export') {
        if (!pwd) {
            showToast('Please enter a password', 'error');
            return;
        }
        if (pwd !== dom.sshPassConfirmInput.value) {
            showToast('Passwords do not match', 'error');
            return;
        }
    } else if (!pwd) {
        showToast('Please enter the password', 'error');
        return;
    }
    dom.sshPassModalEl.classList.add('hidden');
    if (sshPassResolve) {
        sshPassResolve(pwd);
        sshPassResolve = null;
    }
}

function pickSSHExportFile() {
    return new Promise((resolve) => {
        const input = document.createElement('input');
        input.type = 'file';
        input.accept = '.pmuxssh';
        input.onchange = () => {
            resolve((input.files && input.files[0]) || null);
        };
        input.click();
    });
}

function fileToBase64(file) {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result).split(',')[1] || '');
        reader.onerror = () => reject(reader.error);
        reader.readAsDataURL(file);
    });
}

function downloadSSHExport(base64Data, filename) {
    const binary = atob(base64Data);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
    }
    const blob = new Blob([bytes], { type: 'application/octet-stream' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = filename;
    a.click();
    URL.revokeObjectURL(a.href);
}

async function doExportSSH(password) {
    if (!password) return;
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ExportSSH) {
            await window.go.main.App.ExportSSH(password);
        } else {
            const res = await apiPost('/api/ssh/export', { password });
            if (!res || !res.data) throw new Error('Export returned no data');
            downloadSSHExport(res.data, 'pmux-ssh-export.pmuxssh');
        }
        showToast('SSH addresses exported', 'success');
    } catch (e) {
        showToast('Export failed: ' + (e.message || e), 'error');
    }
}

async function doImportSSH(password) {
    if (!password) return;
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ImportSSH) {
            await window.go.main.App.ImportSSH(password);
        } else {
            const file = await pickSSHExportFile();
            if (!file) return;
            const data = await fileToBase64(file);
            await apiPost('/api/ssh/import', { password, data });
        }
        const loaded = await loadSSHConfig();
        if (loaded) sshConfig = loaded;
        if (dom.sshClientPathInput) {
            dom.sshClientPathInput.value = (sshConfig && sshConfig.clientPath) || DEFAULT_SSH_CLIENT_PATH;
        }
        renderSSHAddressList();
        showToast('SSH addresses imported', 'success');
    } catch (e) {
        const msg = (e && e.message) || String(e);
        if (/password|decrypt/i.test(msg)) {
            showToast('Import failed: incorrect password', 'error');
        } else if (/version/i.test(msg)) {
            showToast('Import failed: incompatible export file version', 'error');
        } else {
            showToast('Import failed: ' + msg, 'error');
        }
    }
}

export function initSshEvents() {
    // SSH Manager
    addClick('btn-close-ssh-modal', closeSSHManager);
    addClick('btn-browse-ssh-client', async () => {
        try {
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.SelectFile) {
                const filePath = await window.go.main.App.SelectFile();
                if (filePath) {
                    dom.sshClientPathInput.value = filePath;
                    await saveSSHClientPath();
                }
            }
        } catch (e) {
            console.error('File dialog error:', e);
        }
    });
    addClick('btn-add-ssh-address', () => openSSHAddressModal(null));
    if (dom.sshClientPathInput) {
        dom.sshClientPathInput.addEventListener('change', async () => {
            await saveSSHClientPath();
        });
    }
    addClick('btn-export-ssh', () => {
        openSSHPasswordModal('export', 'Export SSH Addresses', 'Enter a password to encrypt the export file.')
            .then(pwd => doExportSSH(pwd))
            .catch(() => {});
    });
    addClick('btn-import-ssh', () => {
        openSSHPasswordModal('import', 'Import SSH Addresses', 'Enter the password of the export file.')
            .then(pwd => doImportSSH(pwd))
            .catch(() => {});
    });
    addClick('btn-close-ssh-address-modal', closeSSHAddressModal);
    addClick('btn-cancel-ssh-address', closeSSHAddressModal);
    addClick('btn-save-ssh-address', saveSSHAddressFromModal);
    addClick('btn-close-ssh-pass-modal', cancelSSHPasswordModal);
    addClick('btn-cancel-ssh-pass', cancelSSHPasswordModal);
    addClick('btn-ok-ssh-pass', confirmSSHPasswordModal);
    if (dom.sshPassInput) {
        dom.sshPassInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                confirmSSHPasswordModal();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                cancelSSHPasswordModal();
            }
        });
    }
    if (dom.sshPassConfirmInput) {
        dom.sshPassConfirmInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                confirmSSHPasswordModal();
            }
        });
    }
    if (dom.sshAddressModalEl) {
        dom.sshAddressModalEl.addEventListener('click', (e) => {
            if (e.target === dom.sshAddressModalEl) {
                closeSSHAddressModal();
            }
        });
    }
    if (dom.sshModalEl) {
        dom.sshModalEl.addEventListener('click', (e) => {
            if (e.target === dom.sshModalEl) {
                closeSSHManager();
            }
        });
    }
}
