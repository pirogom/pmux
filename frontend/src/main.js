import '@xterm/xterm/css/xterm.css';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import sshWhiteIcon from './assets/images/ssh-white.svg';

// State Management
let serverPort = 4799;
let activeSession = null;
let activePaneId = null;
let sessions = [];
let profiles = [];
let detectedPresets = [];
let activePanesMap = new Map(); // paneId -> { term, fitAddon, ws, element }
let gitPollTimer = null;

// DOM Elements
const sessionListEl = document.getElementById('session-list');
const profileListEl = document.getElementById('profile-list');
const activeSessionTitleEl = document.getElementById('active-session-title');
const btnRenameSessionEl = document.getElementById('btn-rename-session');
const btnRefreshSessionPanesEl = document.getElementById('btn-refresh-session-panes');
const terminalWorkspaceEl = document.getElementById('terminal-container');
const emptyStateEl = document.getElementById('empty-state');

const gitPanelEl = document.getElementById('git-panel');
const gitBranchBadgeEl = document.getElementById('git-branch-badge');
const gitChangesContainerEl = document.getElementById('git-changes-container');
const gitLogOutputEl = document.getElementById('git-log-output');

const profileModalEl = document.getElementById('profile-modal');
const profNameInput = document.getElementById('prof-name');
const profCmdInput = document.getElementById('prof-cmd');
const profArgsInput = document.getElementById('prof-args');
const profDirInput = document.getElementById('prof-dir');
const presetButtonsEl = document.getElementById('preset-buttons');

const confirmModalEl = document.getElementById('confirm-modal');
const confirmTitleEl = document.getElementById('confirm-title');
const confirmMessageEl = document.getElementById('confirm-message');
const btnConfirmOk = document.getElementById('btn-confirm-ok');
const btnConfirmCancel = document.getElementById('btn-confirm-cancel');

const shutdownModalEl = document.getElementById('shutdown-modal');
const shutdownConfirmInput = document.getElementById('shutdown-confirm-input');
const btnConfirmShutdown = document.getElementById('btn-confirm-shutdown');
const btnCancelShutdown = document.getElementById('btn-cancel-shutdown');
const btnCloseShutdownModal = document.getElementById('btn-close-shutdown-modal');
const btnShutdownServer = document.getElementById('btn-shutdown-server');

const toastContainerEl = document.getElementById('toast-container');

const sshModalEl = document.getElementById('ssh-modal');
const sshClientPathInput = document.getElementById('ssh-client-path');
const sshAddressListEl = document.getElementById('ssh-address-list');
const sshAddressModalEl = document.getElementById('ssh-address-modal');
const sshAddrNameInput = document.getElementById('ssh-addr-name');
const sshAddrDescInput = document.getElementById('ssh-addr-desc');
const sshAddrHostInput = document.getElementById('ssh-addr-host');
const sshAddrUserInput = document.getElementById('ssh-addr-user');
const sshPassModalEl = document.getElementById('ssh-pass-modal');
const sshPassTitleEl = document.getElementById('ssh-pass-title');
const sshPassMessageEl = document.getElementById('ssh-pass-message');
const sshPassInput = document.getElementById('ssh-pass-input');
const sshPassConfirmGroupEl = document.getElementById('ssh-pass-confirm-group');
const sshPassConfirmInput = document.getElementById('ssh-pass-confirm-input');

// SSH Manager State
let sshConfig = null;
let sshPaneId = null;
let editingSshAddressId = null;
let sshPassMode = 'export'; // 'export' | 'import'
let sshPassResolve = null;

// Initialization
let isAppInitialized = false;

window.addEventListener('DOMContentLoaded', async () => {
    setupEventListeners();

    const initConfig = async () => {
        if (isAppInitialized) return;
        try {
            if (window.go && window.go.main && window.go.main.App) {
                serverPort = await window.go.main.App.GetServerPort();
                detectedPresets = await window.go.main.App.GetDetectedProfiles();
                isAppInitialized = true;
            }
        } catch (e) {
            console.warn('Wails bindings info:', e);
        }
        await refreshConfigAndProfiles();
        await refreshSessions();
        connectGlobalEventsWS();
    };

    await initConfig();

    // Retry once after 200ms only if Wails IPC bindings were not ready on initial load
    if (!isAppInitialized) {
        setTimeout(initConfig, 200);
    }

    // Initial Git Status poll timer (default 3 seconds)
    startGitPollTimer(3);
});

function startGitPollTimer(seconds) {
    if (gitPollTimer) {
        clearInterval(gitPollTimer);
    }
    const sec = parseInt(seconds, 10) || 3;
    gitPollTimer = setInterval(updateGitStatus, sec * 1000);
}

// Custom UI Components: Toast Notification & Custom Confirm Modal
function showToast(message, type = 'info') {
    // Log to console as well
    if (type === 'error') {
        console.error(`[pmux error] ${message}`);
    } else if (type === 'warning') {
        console.warn(`[pmux warn] ${message}`);
    } else {
        console.log(`[pmux info] ${message}`);
    }

    if (!toastContainerEl) return;
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    const icon = type === 'error' ? '❌' : type === 'success' ? '✅' : 'ℹ️';
    toast.innerHTML = `<span>${icon}</span> <span>${message}</span>`;
    toastContainerEl.appendChild(toast);

    setTimeout(() => {
        toast.style.animation = 'fadeOut 0.3s forwards';
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}

let eventsWS = null;

function connectGlobalEventsWS() {
    if (eventsWS) {
        try { eventsWS.close(); } catch(e) {}
    }
    const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//127.0.0.1:${serverPort}/ws/events`;
    eventsWS = new WebSocket(wsUrl);

    eventsWS.onopen = () => {
        console.log('[events ws] Connected to global events stream');
        refreshSessions();
        refreshConfigAndProfiles();
    };

    eventsWS.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (data.type === 'server_shutdown') {
                console.log('[events ws] Server shutting down. Quitting client...');
                if (window.go && window.go.main && window.go.main.App && window.go.main.App.QuitApp) {
                    window.go.main.App.QuitApp();
                } else {
                    window.close();
                }
            } else if (data.type === 'session_updated') {
                const payload = data.data || {};

                // If it's just an active-pane focus change, do NOT tear down or redraw workspace!
                if (payload.action === 'active-pane') {
                    if (activeSession && payload.sessionId === activeSession.id) {
                        activeSession.activePaneId = payload.paneId;
                        if (activePaneId !== payload.paneId) {
                            setActivePane(payload.paneId, false);
                        }
                    }
                    return;
                }

                // If session was renamed, just refresh title/list without recreating terminal DOM
                if (payload.action === 'renamed') {
                    refreshSessions();
                    return;
                }

                refreshSessions().then(() => {
                    if (activeSession) {
                        const latestSess = sessions.find(s => s.id === activeSession.id);
                        if (latestSess) {
                            attachToSession(latestSess.id);
                        } else if (sessions.length > 0) {
                            attachToSession(sessions[0].id);
                        } else {
                            showEmptyState();
                        }
                    } else if (sessions.length > 0) {
                        attachToSession(sessions[0].id);
                    } else {
                        showEmptyState();
                    }
                });
            } else if (data.type === 'profiles_updated' || data.type === 'config_updated') {
                refreshConfigAndProfiles();
            }
        } catch (e) {
            console.error('[events ws] err:', e);
        }
    };

    eventsWS.onclose = () => {
        setTimeout(connectGlobalEventsWS, 1000);
    };

    eventsWS.onerror = (err) => {
        console.error('[events ws] error:', err);
    };
}

function fallbackCopyTextToClipboard(text) {
    const textArea = document.createElement('textarea');
    textArea.value = text;
    textArea.style.top = '0';
    textArea.style.left = '0';
    textArea.style.position = 'fixed';
    textArea.style.opacity = '0';
    document.body.appendChild(textArea);
    textArea.focus();
    textArea.select();
    try {
        const successful = document.execCommand('copy');
        if (successful) {
            showToast('Copied to clipboard', 'info');
        } else {
            showToast('Failed to copy text', 'error');
        }
    } catch (err) {
        showToast('Failed to copy text: ' + err, 'error');
    }
    document.body.removeChild(textArea);
}

function showConfirm(title, message) {
    return new Promise((resolve) => {
        confirmTitleEl.textContent = title;
        confirmMessageEl.textContent = message;
        confirmModalEl.classList.remove('hidden');

        const onOk = () => {
            cleanup();
            resolve(true);
        };
        const onCancel = () => {
            cleanup();
            resolve(false);
        };
        const cleanup = () => {
            confirmModalEl.classList.add('hidden');
            btnConfirmOk.removeEventListener('click', onOk);
            btnConfirmCancel.removeEventListener('click', onCancel);
        };

        btnConfirmOk.addEventListener('click', onOk);
        btnConfirmCancel.addEventListener('click', onCancel);
    });
}

function openShutdownModal() {
    if (!shutdownModalEl) return;
    shutdownModalEl.classList.remove('hidden');
    if (shutdownConfirmInput) {
        shutdownConfirmInput.value = '';
        setTimeout(() => shutdownConfirmInput.focus(), 50);
    }
    if (btnConfirmShutdown) {
        btnConfirmShutdown.disabled = true;
    }
}

function closeShutdownModal() {
    if (!shutdownModalEl) return;
    shutdownModalEl.classList.add('hidden');
    if (shutdownConfirmInput) {
        shutdownConfirmInput.value = '';
    }
}

async function executeShutdown() {
    if (shutdownConfirmInput && shutdownConfirmInput.value.trim() !== 'shutdown') {
        return;
    }
    closeShutdownModal();
    showToast('Shutting down pmux server and clients...', 'info');
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.KillServer) {
            await window.go.main.App.KillServer();
        } else {
            await fetch(`http://127.0.0.1:${serverPort}/api/server/kill`, { method: 'POST' });
        }
    } catch (err) {
        console.warn('KillServer error / already closing:', err);
    }
}

function addClick(id, handler) {
    const el = document.getElementById(id);
    if (el) {
        el.addEventListener('click', handler);
    }
}

function setupEventListeners() {
    // Ctrl key tracking for terminal pane close button visibility/activation
    const updateCtrlState = (isCtrl) => {
        document.body.classList.toggle('ctrl-pressed', isCtrl);
    };

    window.addEventListener('keydown', (e) => {
        if (e.key === 'Control' || e.ctrlKey) {
            updateCtrlState(true);
        }
    }, true);

    window.addEventListener('keyup', (e) => {
        if (e.key === 'Control' || !e.ctrlKey) {
            updateCtrlState(false);
        }
    }, true);

    window.addEventListener('mousemove', (e) => {
        updateCtrlState(e.ctrlKey);
    }, true);

    window.addEventListener('blur', () => {
        updateCtrlState(false);
    });

    // Automatically sync and refresh terminal panes when window gains focus or becomes visible
    window.addEventListener('focus', () => {
        reflowAllPanes(true, false);
        setTimeout(() => reflowAllPanes(true, false), 80);
    });

    document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible') {
            reflowAllPanes(true, false);
            setTimeout(() => reflowAllPanes(true, false), 80);
        }
    });

    window.addEventListener('resize', () => {
        reflowAllPanes(true, false);
    });

    const sidebarEl = document.getElementById('sidebar');
    const btnToggleTop = document.getElementById('btn-toggle-sidebar-top');
    if (btnToggleTop && sidebarEl) {
        btnToggleTop.addEventListener('click', () => {
            sidebarEl.classList.toggle('collapsed');
            const isCollapsed = sidebarEl.classList.contains('collapsed');
            btnToggleTop.textContent = isCollapsed ? '▶' : '◀';
            if (!isCollapsed) {
                const savedWidth = localStorage.getItem('pmux_sidebar_width');
                if (savedWidth) {
                    const parsed = parseInt(savedWidth, 10);
                    if (!isNaN(parsed) && parsed >= 160 && parsed <= 600) {
                        sidebarEl.style.width = `${parsed}px`;
                    }
                }
            }
            
            // Dispatch resize to adjust xterm.js terminal viewport
            setTimeout(() => {
                reflowAllPanes();
                window.dispatchEvent(new Event('resize'));
            }, 220);
        });
    }

    initSidebarResizer();
    initCollapsibleSidebarSections();

    addClick('btn-new-session-quick', () => {
        if (profiles.length > 0) {
            createSessionFromProfile(profiles[0]);
        } else {
            showToast('Please create a profile first', 'info');
            openProfileModal();
        }
    });

    addClick('btn-split-v', () => {
        if (activeSession && activePaneId) {
            splitCurrentPane('vertical');
        } else {
            showToast('No active pane to split', 'error');
        }
    });

    addClick('btn-split-h', () => {
        if (activeSession && activePaneId) {
            splitCurrentPane('horizontal');
        } else {
            showToast('No active pane to split', 'error');
        }
    });

    addClick('btn-toggle-git', () => {
        gitPanelEl.classList.toggle('hidden');
        if (!gitPanelEl.classList.contains('hidden')) {
            updateGitStatus();
        }
    });

    addClick('btn-close-git', () => {
        gitPanelEl.classList.add('hidden');
    });

    addClick('btn-git-refresh', updateGitStatus);
    initGitPanelResizer();

    const pollSelectEl = document.getElementById('git-poll-interval-select');
    if (pollSelectEl) {
        pollSelectEl.addEventListener('change', async (e) => {
            const newInterval = parseInt(e.target.value, 10) || 3;
            startGitPollTimer(newInterval);
            try {
                if (window.go && window.go.main && window.go.main.App && window.go.main.App.SaveGitPollInterval) {
                    await window.go.main.App.SaveGitPollInterval(newInterval);
                } else {
                    await apiPost('/api/config/git-poll-interval', { interval: newInterval });
                }
                showToast(`Git refresh interval set to ${newInterval}s`, 'info');
            } catch (err) {
                console.error('Failed to save git poll interval:', err);
            }
        });
    }

    addClick('btn-rename-session', () => startEditingSessionTitle());
    addClick('btn-refresh-session-panes', () => {
        if (!activeSession) return;
        reflowAllPanes(true, true);
        setTimeout(() => reflowAllPanes(true, true), 60);
        showToast('Session panes refreshed', 'info');
    });

    if (activeSessionTitleEl) {
        activeSessionTitleEl.addEventListener('click', () => startEditingSessionTitle());
        activeSessionTitleEl.addEventListener('input', () => {
            const raw = activeSessionTitleEl.innerText || activeSessionTitleEl.textContent || '';
            if (raw.length > 256) {
                const trimmed = raw.slice(0, 256);
                activeSessionTitleEl.textContent = trimmed;
                try {
                    const range = document.createRange();
                    range.selectNodeContents(activeSessionTitleEl);
                    range.collapse(false);
                    const sel = window.getSelection();
                    sel.removeAllRanges();
                    sel.addRange(range);
                } catch (e) {}
            }
        });
        activeSessionTitleEl.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                activeSessionTitleEl.blur();
            } else if (e.key === 'Escape') {
                finishEditingSessionTitle(false);
            }
        });
        activeSessionTitleEl.addEventListener('blur', () => finishEditingSessionTitle(true));
    }

    addClick('btn-add-profile', () => openProfileModal());

    addClick('btn-browse-cmd', async () => {
        try {
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.SelectFile) {
                const filePath = await window.go.main.App.SelectFile();
                if (filePath) {
                    document.getElementById('prof-cmd').value = filePath;
                }
            }
        } catch (e) {
            console.error('File dialog error:', e);
        }
    });

    addClick('btn-browse-dir', async () => {
        try {
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.SelectDirectory) {
                const dirPath = await window.go.main.App.SelectDirectory();
                if (dirPath) {
                    document.getElementById('prof-dir').value = dirPath;
                }
            }
        } catch (e) {
            console.error('Directory dialog error:', e);
        }
    });

    addClick('btn-close-modal', closeProfileModal);
    addClick('btn-cancel-profile', closeProfileModal);
    addClick('btn-save-profile', saveProfileFromModal);
    addClick('btn-refresh-sessions', refreshSessions);

    // SSH Manager
    addClick('btn-close-ssh-modal', closeSSHManager);
    addClick('btn-browse-ssh-client', async () => {
        try {
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.SelectFile) {
                const filePath = await window.go.main.App.SelectFile();
                if (filePath) {
                    sshClientPathInput.value = filePath;
                    await saveSSHClientPath();
                }
            }
        } catch (e) {
            console.error('File dialog error:', e);
        }
    });
    addClick('btn-add-ssh-address', () => openSSHAddressModal(null));
    if (sshClientPathInput) {
        sshClientPathInput.addEventListener('change', async () => {
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
    if (sshPassInput) {
        sshPassInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                confirmSSHPasswordModal();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                cancelSSHPasswordModal();
            }
        });
    }
    if (sshPassConfirmInput) {
        sshPassConfirmInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                confirmSSHPasswordModal();
            }
        });
    }
    if (sshAddressModalEl) {
        sshAddressModalEl.addEventListener('click', (e) => {
            if (e.target === sshAddressModalEl) {
                closeSSHAddressModal();
            }
        });
    }
    if (sshModalEl) {
        sshModalEl.addEventListener('click', (e) => {
            if (e.target === sshModalEl) {
                closeSSHManager();
            }
        });
    }

    addClick('btn-shutdown-server', openShutdownModal);
    addClick('btn-cancel-shutdown', closeShutdownModal);
    addClick('btn-close-shutdown-modal', closeShutdownModal);

    if (shutdownConfirmInput) {
        shutdownConfirmInput.addEventListener('input', (e) => {
            if (btnConfirmShutdown) {
                btnConfirmShutdown.disabled = (e.target.value.trim() !== 'shutdown');
            }
        });
        shutdownConfirmInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && shutdownConfirmInput.value.trim() === 'shutdown') {
                e.preventDefault();
                executeShutdown();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                closeShutdownModal();
            }
        });
    }

    if (btnConfirmShutdown) {
        btnConfirmShutdown.addEventListener('click', executeShutdown);
    }

    if (shutdownModalEl) {
        shutdownModalEl.addEventListener('click', (e) => {
            if (e.target === shutdownModalEl) {
                closeShutdownModal();
            }
        });
    }
}

// API Calls (Server HTTP API Fallback)
async function apiGet(path) {
    const res = await fetch(`http://127.0.0.1:${serverPort}${path}`);
    return await res.json();
}

async function apiPost(path, body) {
    const res = await fetch(`http://127.0.0.1:${serverPort}${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    });
    return await res.json();
}

// Session Management
async function refreshSessions() {
    try {
        // Query central daemon server REST API first to guarantee 0.01s real-time multi-client sync
        try {
            sessions = await apiGet('/api/sessions');
        } catch (e) {
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetSessions) {
                sessions = await window.go.main.App.GetSessions();
            } else {
                throw e;
            }
        }

        if (activeSession) {
            const updatedActiveSess = sessions.find(s => s.id === activeSession.id);
            if (updatedActiveSess) {
                activeSession = updatedActiveSess;
                if (!isEditingSessionTitle) {
                    setSessionTitle(activeSession.name);
                }
            } else if (sessions.length === 0) {
                showEmptyState();
            }
        } else if (sessions.length > 0) {
            attachToSession(sessions[0].id);
        }

        renderSessionList();
    } catch (e) {
        console.error('Failed to refresh sessions:', e);
    }
}

function renderSessionList() {
    sessionListEl.innerHTML = '';
    if (!sessions || sessions.length === 0) {
        const emptyLi = document.createElement('li');
        emptyLi.textContent = 'No sessions';
        emptyLi.style.color = 'var(--text-muted)';
        emptyLi.style.cursor = 'default';
        sessionListEl.appendChild(emptyLi);
        return;
    }

    sessions.forEach(sess => {
        const li = document.createElement('li');
        li.title = sess.name;
        li.innerHTML = `<span>💻 ${sess.name}</span><button class="icon-btn-small del-btn" title="Close Session">✖</button>`;
        if (activeSession && activeSession.id === sess.id) {
            li.classList.add('active');
        }
        li.addEventListener('click', () => attachToSession(sess.id));
        li.querySelector('.del-btn').addEventListener('click', async (e) => {
            e.stopPropagation();
            await closeSession(sess.id, sess.name);
        });
        sessionListEl.appendChild(li);
    });
}

async function closeSession(sessionId, sessionName) {
    const confirmed = await showConfirm('Close Session', `Are you sure you want to close "${sessionName}"?`);
    if (!confirmed) return;

    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.CloseSession) {
            await window.go.main.App.CloseSession(sessionId);
        } else {
            await apiPost('/api/sessions/close-session', { sessionId });
        }

        // Clean active panes map for closed session
        const sess = sessions.find(s => s.id === sessionId);
        if (sess && sess.panes) {
            Object.keys(sess.panes).forEach(paneId => {
                if (activePanesMap.has(paneId)) {
                    const p = activePanesMap.get(paneId);
                    try { p.ws.close(); } catch(e){}
                    activePanesMap.delete(paneId);
                }
            });
        }

        if (activeSession && activeSession.id === sessionId) {
            activeSession = null;
            activePaneId = null;
            terminalWorkspaceEl.innerHTML = '';
            emptyStateEl.style.display = 'flex';
            activeSessionTitleEl.textContent = 'No Active Session';
            if (btnRenameSessionEl) btnRenameSessionEl.classList.add('hidden');
            if (btnRefreshSessionPanesEl) btnRefreshSessionPanesEl.classList.add('hidden');
        }

        await refreshSessions();
        showToast(`Closed session: ${sessionName}`, 'success');
    } catch (e) {
        showToast(`Failed to close session: ${e.message || e}`, 'error');
    }
}

function generateSessionName(profName) {
    const baseName = profName;
    const existingNames = sessions.map(s => s.name);

    if (!existingNames.includes(baseName)) {
        return baseName;
    }

    let num = 2;
    while (existingNames.includes(`${baseName} #${num}`)) {
        num++;
    }
    return `${baseName} #${num}`;
}

function setSessionTitle(name) {
    if (!activeSessionTitleEl) return;
    activeSessionTitleEl.textContent = name;
    activeSessionTitleEl.title = name;
    activeSessionTitleEl.scrollLeft = 0;
}

let isEditingSessionTitle = false;

function startEditingSessionTitle() {
    if (!activeSession || isEditingSessionTitle) return;
    isEditingSessionTitle = true;
    activeSessionTitleEl.contentEditable = "true";
    activeSessionTitleEl.focus();

    try {
        const range = document.createRange();
        range.selectNodeContents(activeSessionTitleEl);
        const sel = window.getSelection();
        sel.removeAllRanges();
        sel.addRange(range);
    } catch (e) {}
}

async function finishEditingSessionTitle(commit = true) {
    if (!isEditingSessionTitle) return;
    isEditingSessionTitle = false;
    activeSessionTitleEl.contentEditable = "false";
    activeSessionTitleEl.scrollLeft = 0;

    if (!activeSession) return;

    const rawText = activeSessionTitleEl.innerText || activeSessionTitleEl.textContent || '';
    let newName = rawText.replace(/\u00a0/g, ' ').replace(/[\r\n]+/g, ' ').trim();
    if (newName.length > 256) {
        newName = newName.slice(0, 256).trim();
    }
    if (!commit || !newName || newName === activeSession.name) {
        setSessionTitle(activeSession.name);
        return;
    }

    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.RenameSession) {
            await window.go.main.App.RenameSession(activeSession.id, newName);
        } else {
            await apiPost('/api/sessions/rename', { sessionId: activeSession.id, newName });
        }
        activeSession.name = newName;
        setSessionTitle(newName);
        await refreshSessions();
        showToast(`Session renamed to "${newName}"`, 'info');
    } catch (e) {
        setSessionTitle(activeSession ? activeSession.name : 'No Active Session');
        showToast('Failed to rename session: ' + (e.message || e), 'error');
    }
}



async function createSessionFromProfile(prof) {
    try {
        const sessionName = generateSessionName(prof.name);
        const req = {
            profileId: prof.id || prof.name,
            name: sessionName,
            command: prof.command,
            args: prof.args || [],
            workDir: prof.workDir || '',
            cols: 100,
            rows: 30
        };

        let data = null;
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.CreateSession) {
            data = await window.go.main.App.CreateSession(req);
        } else {
            data = await apiPost('/api/sessions', req);
        }

        await refreshSessions();
        if (data && data.session) {
            attachToSession(data.session.id);
            showToast(`Started session: ${sessionName}`, 'success');
        }
    } catch (e) {
        showToast(`Failed to create session: ${e.message || e}`, 'error');
    }
}

function cleanupOrphanPanes() {
    // Collect all valid pane IDs across all active sessions
    const validPaneIds = new Set();
    sessions.forEach(s => {
        if (s && s.panes) {
            Object.keys(s.panes).forEach(id => validPaneIds.add(id));
        }
    });

    activePanesMap.forEach((entry, paneId) => {
        if (!validPaneIds.has(paneId)) {
            // Pane was closed or no longer exists in any session
            try {
                if (entry.ws) {
                    entry.ws.onclose = null; // Prevent reconnect
                    entry.ws.close();
                }
                if (entry.term) {
                    entry.term.dispose();
                }
                if (entry.element) {
                    entry.element.remove();
                }
            } catch (e) {}
            activePanesMap.delete(paneId);
        }
    });
}

function showEmptyState() {
    activeSession = null;
    activePaneId = null;
    setSessionTitle('No Active Session');
    if (btnRenameSessionEl) btnRenameSessionEl.classList.add('hidden');
    if (btnRefreshSessionPanesEl) btnRefreshSessionPanesEl.classList.add('hidden');
    if (terminalWorkspaceEl) terminalWorkspaceEl.innerHTML = '';
    if (emptyStateEl) emptyStateEl.style.display = 'flex';
    renderSessionList();
}

async function attachToSession(sessionId) {
    const sess = sessions.find(s => s.id === sessionId);
    if (!sess) {
        if (sessions.length > 0) {
            attachToSession(sessions[0].id);
        } else {
            showEmptyState();
        }
        return;
    }

    activeSession = sess;
    setSessionTitle(sess.name);
    if (btnRenameSessionEl) btnRenameSessionEl.classList.remove('hidden');
    if (btnRefreshSessionPanesEl) btnRefreshSessionPanesEl.classList.remove('hidden');
    renderSessionList();

    // Cleanup any deleted/orphan panes from activePanesMap (across all sessions)
    cleanupOrphanPanes();

    // Render Terminal Grid layout
    terminalWorkspaceEl.innerHTML = '';
    emptyStateEl.style.display = 'none';

    const layoutContainer = renderLayoutTree(sess.layout, sess);
    terminalWorkspaceEl.appendChild(layoutContainer);

    // Set focus to active pane
    if (sess.activePaneId && activePanesMap.has(sess.activePaneId)) {
        setActivePane(sess.activePaneId, false);
    }
    
    // Coordinated reflow pass to calculate layout and synchronize ConPTY geometry
    requestAnimationFrame(() => {
        reflowAllPanes(true, false);
    });
}

function renderLayoutTree(node, sess, isRoot = true) {
    if (!node) return document.createElement('div');

    if (node.id) {
        // Leaf Node -> Terminal Pane
        const paneEl = document.createElement('div');
        paneEl.className = 'pane-element';
        paneEl.id = `pane-box-${node.id}`;
        if (!isRoot) {
            paneEl.style.flex = `${node.ratio || 1} 1 0%`;
        } else {
            paneEl.style.flex = '1 1 100%';
            paneEl.style.height = '100%';
            paneEl.style.width = '100%';
        }
        paneEl.addEventListener('click', () => {
            setActivePane(node.id);
        });

        const paneData = sess.panes[node.id];
        if (paneData) {
            initXtermPane(paneEl, paneData);
        }

        // Pane Extension Toolbar (Visible ONLY when Ctrl key is pressed)
        const paneToolbar = document.createElement('div');
        paneToolbar.className = 'pane-toolbar';

        const sshToolbarBtn = document.createElement('button');
        sshToolbarBtn.className = 'pane-toolbar-btn';
        sshToolbarBtn.title = 'SSH Manager (Ctrl + Click)';
        const sshToolbarImg = document.createElement('img');
        sshToolbarImg.src = sshWhiteIcon;
        sshToolbarImg.alt = 'SSH';
        sshToolbarBtn.appendChild(sshToolbarImg);
        sshToolbarBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            if (!e.ctrlKey && !document.body.classList.contains('ctrl-pressed')) {
                return;
            }
            openSSHManager(sess.id, node.id);
        });
        paneToolbar.appendChild(sshToolbarBtn);
        paneEl.appendChild(paneToolbar);

        // Close Pane Button (Visible and enabled ONLY when Ctrl key is pressed)
        const closeBtn = document.createElement('button');
        closeBtn.className = 'pane-close-btn';
        closeBtn.textContent = '✖';
        closeBtn.title = 'Close Terminal (Ctrl + Click)';
        closeBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            if (!e.ctrlKey && !document.body.classList.contains('ctrl-pressed')) {
                return;
            }
            const confirmed = await showConfirm('Close Terminal', 'Are you sure you want to close this terminal pane?');
            if (confirmed) {
                await closePane(sess.id, node.id);
            }
        });
        paneEl.appendChild(closeBtn);

        return paneEl;
    }

    // Branch Node -> Split Container
    const container = document.createElement('div');
    container.className = `pane-container ${node.direction}`;
    if (!isRoot) {
        container.style.flex = `${node.ratio || 1} 1 0%`;
    } else {
        container.style.flex = '1 1 100%';
        container.style.height = '100%';
        container.style.width = '100%';
    }

    if (node.children && node.children.length > 0) {
        node.children.forEach((childNode, index) => {
            if (index > 0) {
                // Insert Resizable Splitter Handle between child nodes
                const splitter = document.createElement('div');
                splitter.className = 'pane-splitter';
                makeSplitterResizable(splitter, container, node.direction, node.children[index - 1], childNode);
                container.appendChild(splitter);
            }
            const childEl = renderLayoutTree(childNode, sess, false);
            container.appendChild(childEl);
        });
    }
    return container;
}

function makeSplitterResizable(splitter, container, direction, prevNode, nextNode) {
    let startX = 0, startY = 0;
    let prevStartSize = 0, nextStartSize = 0;

    const onMouseDown = (e) => {
        e.preventDefault();
        e.stopPropagation();

        splitter.classList.add('dragging');
        startX = e.clientX;
        startY = e.clientY;

        const prevEl = splitter.previousElementSibling;
        const nextEl = splitter.nextElementSibling;
        if (!prevEl || !nextEl) return;

        const isVertical = (direction === 'vertical'); // Left-Right split
        prevStartSize = isVertical ? prevEl.getBoundingClientRect().width : prevEl.getBoundingClientRect().height;
        nextStartSize = isVertical ? nextEl.getBoundingClientRect().width : nextEl.getBoundingClientRect().height;
        const totalSize = prevStartSize + nextStartSize;

        const onMouseMove = (moveEvent) => {
            const delta = isVertical ? (moveEvent.clientX - startX) : (moveEvent.clientY - startY);
            let newPrevSize = prevStartSize + delta;
            let newNextSize = nextStartSize - delta;

            if (newPrevSize < 40) newPrevSize = 40;
            if (newNextSize < 40) newNextSize = 40;

            const prevRatio = newPrevSize / totalSize;
            const nextRatio = newNextSize / totalSize;

            prevEl.style.flex = `${prevRatio} 1 0%`;
            nextEl.style.flex = `${nextRatio} 1 0%`;

            prevNode.ratio = prevRatio;
            nextNode.ratio = nextRatio;

            // Trigger xterm fit reflow while dragging
            reflowAllPanes();
        };

        const onMouseUp = () => {
            splitter.classList.remove('dragging');
            window.removeEventListener('mousemove', onMouseMove);
            window.removeEventListener('mouseup', onMouseUp);

            reflowAllPanes();
        };

        window.addEventListener('mousemove', onMouseMove);
        window.addEventListener('mouseup', onMouseUp);
    };

    splitter.addEventListener('mousedown', onMouseDown);
}

function initCollapsibleSidebarSections() {
    const headerSessions = document.getElementById('header-sessions');
    const sectionSessions = document.getElementById('section-sessions');
    const headerProfiles = document.getElementById('header-profiles');
    const sectionProfiles = document.getElementById('section-profiles');
    const btnAddProfile = document.getElementById('btn-add-profile');

    const updateSectionsExpandState = () => {
        if (sectionProfiles && sectionSessions) {
            if (sectionProfiles.classList.contains('collapsed')) {
                sectionSessions.classList.add('expand-fill');
            } else {
                sectionSessions.classList.remove('expand-fill');
            }
            if (sectionSessions.classList.contains('collapsed')) {
                sectionProfiles.classList.add('expand-fill');
            } else {
                sectionProfiles.classList.remove('expand-fill');
            }
        }
    };

    // Restore saved collapse states from localStorage
    if (localStorage.getItem('pmux_sessions_collapsed') === 'true' && sectionSessions) {
        sectionSessions.classList.add('collapsed');
    }
    if (localStorage.getItem('pmux_profiles_collapsed') === 'true' && sectionProfiles) {
        sectionProfiles.classList.add('collapsed');
    }
    updateSectionsExpandState();

    if (headerSessions && sectionSessions) {
        headerSessions.addEventListener('click', () => {
            sectionSessions.classList.toggle('collapsed');
            localStorage.setItem('pmux_sessions_collapsed', sectionSessions.classList.contains('collapsed'));
            updateSectionsExpandState();
        });
    }

    if (headerProfiles && sectionProfiles) {
        headerProfiles.addEventListener('click', () => {
            sectionProfiles.classList.toggle('collapsed');
            localStorage.setItem('pmux_profiles_collapsed', sectionProfiles.classList.contains('collapsed'));
            updateSectionsExpandState();
        });
    }

    // Prevent Add Profile button click from triggering section toggle
    if (btnAddProfile) {
        btnAddProfile.addEventListener('click', (e) => {
            e.stopPropagation();
        });
    }
}

function initSidebarResizer() {
    const sidebar = document.getElementById('sidebar');
    const resizer = document.getElementById('sidebar-resizer');
    if (!sidebar || !resizer) return;

    const MIN_WIDTH = 160;
    const MAX_WIDTH = 600;

    // Load saved width
    const savedWidth = localStorage.getItem('pmux_sidebar_width');
    if (savedWidth && !sidebar.classList.contains('collapsed')) {
        const parsed = parseInt(savedWidth, 10);
        if (!isNaN(parsed) && parsed >= MIN_WIDTH && parsed <= MAX_WIDTH) {
            sidebar.style.width = `${parsed}px`;
        }
    }

    let startX = 0;
    let startWidth = 0;

    resizer.addEventListener('mousedown', (e) => {
        if (sidebar.classList.contains('collapsed')) return;
        e.preventDefault();
        startX = e.clientX;
        startWidth = sidebar.getBoundingClientRect().width;

        sidebar.classList.add('resizing');
        resizer.classList.add('dragging');
        document.body.classList.add('resizing-sidebar');

        const onMouseMove = (moveEvent) => {
            const deltaX = moveEvent.clientX - startX;
            let newWidth = startWidth + deltaX;
            if (newWidth < MIN_WIDTH) newWidth = MIN_WIDTH;
            if (newWidth > MAX_WIDTH) newWidth = MAX_WIDTH;

            sidebar.style.width = `${newWidth}px`;
            reflowAllPanes();
        };

        const onMouseUp = () => {
            sidebar.classList.remove('resizing');
            resizer.classList.remove('dragging');
            document.body.classList.remove('resizing-sidebar');

            window.removeEventListener('mousemove', onMouseMove);
            window.removeEventListener('mouseup', onMouseUp);

            const finalWidth = sidebar.getBoundingClientRect().width;
            localStorage.setItem('pmux_sidebar_width', finalWidth);
            reflowAllPanes();
        };

        window.addEventListener('mousemove', onMouseMove);
        window.addEventListener('mouseup', onMouseUp);
    });
}

function initXtermPane(containerEl, paneData) {
    if (activePanesMap.has(paneData.id)) {
        // Re-attach existing term element
        const existing = activePanesMap.get(paneData.id);
        containerEl.appendChild(existing.element);
        try {
            if (existing.term) {
                existing.term.refresh(0, existing.term.rows - 1);
            }
        } catch(e) {}
        return;
    }

    const termBox = document.createElement('div');
    termBox.className = 'xterm-instance';
    containerEl.appendChild(termBox);

    const term = new Terminal({
        fontFamily: '"Cascadia Code", "Fira Code", Consolas, monospace',
        fontSize: 14,
        theme: {
            background: '#0d0e11',
            foreground: '#abb2bf',
            cursor: '#528bff'
        },
        cursorBlink: true
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(termBox);

    try {
        const webglAddon = new WebglAddon();
        term.loadAddon(webglAddon);
    } catch (e) {
        // WebGL fallback to DOM renderer
    }

    setTimeout(() => {
        try {
            fitAddon.fit();
            term.focus();
        } catch (e) {}
    }, 100);

    // WebSocket connection to ConPTY stream with 3-retry 1s-delay management
    let retryCount = 0;
    let hasEverConnected = false;
    let isReconnecting = false;
    let isWritingServerOutput = false;

    const connectPaneWS = () => {
        const wsUrl = `ws://127.0.0.1:${serverPort}/ws/pane/${paneData.id}`;
        const ws = new WebSocket(wsUrl);

        if (activePanesMap.has(paneData.id)) {
            activePanesMap.get(paneData.id).ws = ws;
        }

        ws.onopen = () => {
            console.log(`WebSocket connected to pane ${paneData.id} at ${wsUrl}`);
            if (isReconnecting) {
                try {
                    term.reset();
                } catch (e) {}
                showToast('Terminal connection restored', 'success');
                isReconnecting = false;
            }
            hasEverConnected = true;
            retryCount = 0; // Reset retry count on successful connection
            setTimeout(() => {
                try {
                    fitAddon.fit();
                    term.focus();
                } catch (e) {}
            }, 200);
        };

        ws.binaryType = 'arraybuffer';

        ws.onmessage = (event) => {
            if (typeof event.data === 'string') {
                term.write(event.data);
            } else if (event.data instanceof ArrayBuffer) {
                term.write(new Uint8Array(event.data));
            } else if (event.data instanceof Blob) {
                event.data.text().then(text => {
                    if (text) term.write(text);
                });
            }
        };

        ws.onerror = (e) => {
            console.warn(`WebSocket connection error for pane ${paneData.id}:`, e);
        };

        ws.onclose = (event) => {
            // 1. If normal closure (code 1000 or 1001), do not retry
            if (event && (event.code === 1000 || event.code === 1001)) {
                return;
            }

            // 2. Check if pane still exists in active workspace
            if (!activePanesMap.has(paneData.id)) {
                return; // Pane was closed manually, do not retry
            }

            // 3. If WebSocket NEVER successfully connected, this is an initial connect failure (not a dropped active session).
            // Do NOT trigger reconnect toasts or loops during initial startup / dead pane rendering.
            if (!hasEverConnected) {
                console.warn(`Initial WebSocket connection failed for pane ${paneData.id}. Will not auto-reconnect.`);
                return;
            }

            // 4. WebSocket was previously established and dropped unexpectedly -> auto-reconnect
            isReconnecting = true;
            if (retryCount < 3) {
                retryCount++;
                showToast(`Terminal connection lost. Reconnecting (${retryCount}/3)...`, 'info');
                setTimeout(() => {
                    if (activePanesMap.has(paneData.id)) {
                        connectPaneWS();
                    }
                }, 1000);
            } else {
                showToast('Failed to reconnect to terminal pane after 3 retries.', 'error');
                isReconnecting = false;
            }
        };

        return ws;
    };

    const ws = connectPaneWS();

    let inputBatchBuf = '';
    let inputBatchTimer = null;

    const flushInputBatch = () => {
        if (!inputBatchBuf) return;
        const toSend = inputBatchBuf;
        inputBatchBuf = '';
        if (inputBatchTimer) {
            clearTimeout(inputBatchTimer);
            inputBatchTimer = null;
        }
        const paneEntry = activePanesMap.get(paneData.id);
        const currentWS = paneEntry ? paneEntry.ws : ws;
        if (currentWS && currentWS.readyState === WebSocket.OPEN) {
            currentWS.send(JSON.stringify({ type: 'input', data: toSend }));
        }
    };

    term.onData((data) => {
        inputBatchBuf += data;
        if (!inputBatchTimer) {
            inputBatchTimer = setTimeout(flushInputBatch, 4);
        }
    });

    term.attachCustomKeyEventHandler((event) => {
        if (event.type === 'keydown') {
            const isCtrlC = (event.ctrlKey || event.metaKey) && event.code === 'KeyC';
            const isCtrlV = (event.ctrlKey || event.metaKey) && event.code === 'KeyV';

            if (isCtrlC) {
                if (term.hasSelection()) {
                    const selection = term.getSelection();
                    if (selection && selection.length > 0) {
                        if (navigator.clipboard && navigator.clipboard.writeText) {
                            navigator.clipboard.writeText(selection).then(() => {
                                showToast('Copied to clipboard', 'info');
                            }).catch(() => {
                                fallbackCopyTextToClipboard(selection);
                            });
                        } else {
                            fallbackCopyTextToClipboard(selection);
                        }
                        return false;
                    }
                }
                return true;
            }

            // Alt + V: Send literal ^V (\x16, Literal Next / Visual Block) to terminal backend
            if (event.altKey && !event.ctrlKey && !event.metaKey && event.code === 'KeyV') {
                event.preventDefault();
                flushInputBatch();
                const paneEntry = activePanesMap.get(paneData.id);
                const currentWS = paneEntry ? paneEntry.ws : ws;
                if (currentWS && currentWS.readyState === WebSocket.OPEN) {
                    currentWS.send(JSON.stringify({ type: 'input', data: '\x16' }));
                }
                return false;
            }

            if (isCtrlV) {
                // Ctrl + V & Shift + Ctrl + V: Prevent xterm from sending raw \x16 to backend,
                // allowing browser native 'paste' event to handle clipboard paste smoothly.
                return false;
            }
        }
        return true;
    });

    let lastSentCols = 0;
    let lastSentRows = 0;

    term.onResize((size) => {
        if (size.cols < 10 || size.rows < 3) return; // Guard against temporary zero/tiny bounds during drag
        if (size.cols === lastSentCols && size.rows === lastSentRows) {
            return; // Skip duplicate resize message
        }
        lastSentCols = size.cols;
        lastSentRows = size.rows;

        if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows }));
        }
    });

    activePanesMap.set(paneData.id, {
        term,
        fitAddon,
        ws,
        element: termBox,
        workDir: paneData.workDir,
        command: paneData.command,
        args: paneData.args
    });
}

async function closePane(sessionId, paneId) {
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ClosePane) {
            await window.go.main.App.ClosePane(sessionId, paneId);
        } else {
            await apiPost('/api/sessions/close-pane', { sessionId, paneId });
        }
        
        if (activePanesMap.has(paneId)) {
            const paneObj = activePanesMap.get(paneId);
            if (paneObj.ws) paneObj.ws.close();
            activePanesMap.delete(paneId);
        }

        await refreshSessions();
        if (activeSession) {
            attachToSession(activeSession.id);
        } else {
            terminalWorkspaceEl.innerHTML = '';
            emptyStateEl.style.display = 'flex';
        }
        showToast('Pane closed', 'info');
    } catch (e) {
        showToast('Failed to close pane: ' + (e.message || e), 'error');
    }
}

function setActivePane(paneId, notifyServer = true) {
    activePaneId = paneId;
    document.querySelectorAll('.pane-element').forEach(el => el.classList.remove('active'));
    const box = document.getElementById(`pane-box-${paneId}`);
    if (box) box.classList.add('active');

    if (activePanesMap.has(paneId)) {
        const p = activePanesMap.get(paneId);
        p.term.focus();
    }
    updateGitStatus();

    if (notifyServer && activeSession) {
        apiPost('/api/sessions/active-pane', { sessionId: activeSession.id, paneId }).catch(() => {});
    }
}

async function splitCurrentPane(direction) {
    if (!activeSession || !activePaneId) return;

    const parentPaneData = activeSession.panes ? activeSession.panes[activePaneId] : null;
    const currentPane = activePanesMap.get(activePaneId);
    const fallbackProf = profiles[0] || detectedPresets[0] || { command: 'cmd.exe', args: [] };

    const command = (parentPaneData && parentPaneData.command) 
        ? parentPaneData.command 
        : (currentPane && currentPane.command) 
            ? currentPane.command 
            : fallbackProf.command;

    const args = (parentPaneData && parentPaneData.args) 
        ? parentPaneData.args 
        : (currentPane && currentPane.args) 
            ? currentPane.args 
            : (fallbackProf.args || []);

    const workDir = currentPane ? currentPane.workDir : (parentPaneData ? parentPaneData.workDir : '');

    const req = {
        sessionId: activeSession.id,
        parentPaneId: activePaneId,
        direction: direction,
        command: command,
        args: args,
        workDir: workDir,
        cols: 80,
        rows: 24
    };

    try {
        let newPane = null;
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.SplitPane) {
            newPane = await window.go.main.App.SplitPane(req);
        } else {
            newPane = await apiPost('/api/sessions/split', req);
        }

        await refreshSessions();
        if (newPane && newPane.id) {
            attachToSession(activeSession.id);
            setActivePane(newPane.id);
            reflowAllPanes();
            showToast('Pane split successfully', 'success');
        }
    } catch (e) {
        showToast('Failed to split pane: ' + (e.message || e), 'error');
    }
}

let reflowDebounceTimer = null;

function reflowAllPanes(forceSendResize = false, forceRedraw = false) {
    if (reflowDebounceTimer) {
        clearTimeout(reflowDebounceTimer);
    }
    reflowDebounceTimer = setTimeout(() => {
        activePanesMap.forEach((paneObj) => {
            const { fitAddon, term, ws, element } = paneObj;
            try {
                if (element && element.isConnected && element.offsetWidth > 0 && element.offsetHeight > 0) {
                    fitAddon.fit();
                    if (term) {
                        term.refresh(0, term.rows - 1);
                        if (ws && ws.readyState === WebSocket.OPEN && term.cols >= 10 && term.rows >= 3) {
                            if (forceRedraw) {
                                ws.send(JSON.stringify({ type: 'redraw', cols: term.cols, rows: term.rows }));
                            } else if (forceSendResize) {
                                ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
                            }
                        }
                    }
                }
            } catch(e) {}
        });
    }, 30);
}

// Profiles
async function refreshConfigAndProfiles() {
    try {
        let loaded = null;
        // Query central daemon server REST API first to guarantee 0.01s real-time multi-client sync
        try {
            loaded = await apiGet('/api/profiles');
        } catch (e) {
            if (window.go && window.go.main && window.go.main.App) {
                if (window.go.main.App.GetConfig) {
                    const cfg = await window.go.main.App.GetConfig();
                    if (cfg) {
                        if (cfg.gitPollInterval) {
                            const selectEl = document.getElementById('git-poll-interval-select');
                            if (selectEl) selectEl.value = cfg.gitPollInterval;
                            startGitPollTimer(cfg.gitPollInterval);
                        }
                        loaded = cfg.profiles || cfg.Profiles || [];
                    }
                }
                if ((!loaded || loaded.length === 0) && window.go.main.App.GetProfiles) {
                    loaded = await window.go.main.App.GetProfiles();
                }
            }
        }
        profiles = loaded || [];
        console.log('[pmux profiles loaded]', profiles);
        renderProfileList();
    } catch (e) {
        console.error('Failed to load profiles:', e);
    }
}

let editingProfileId = null;

function renderProfileList() {
    profileListEl.innerHTML = '';
    if (!profiles || profiles.length === 0) {
        const emptyLi = document.createElement('li');
        emptyLi.textContent = 'No profiles';
        emptyLi.style.color = 'var(--text-muted)';
        emptyLi.style.cursor = 'default';
        profileListEl.appendChild(emptyLi);
        return;
    }

    profiles.forEach(prof => {
        const li = document.createElement('li');
        li.title = prof.name;
        li.innerHTML = `<span>🐚 ${prof.name}</span>
            <div class="item-actions">
                <button class="icon-btn-small edit-btn" title="Edit Profile">✏️</button>
                <button class="icon-btn-small del-btn" title="Delete Profile">✖</button>
            </div>`;
        li.addEventListener('click', () => createSessionFromProfile(prof));
        li.querySelector('.edit-btn').addEventListener('click', (e) => {
            e.stopPropagation();
            openProfileModal(prof);
        });
        li.querySelector('.del-btn').addEventListener('click', async (e) => {
            e.stopPropagation();
            await deleteProfile(prof.id);
        });
        profileListEl.appendChild(li);
    });
}

async function deleteProfile(id) {
    const confirmed = await showConfirm('Delete Profile', 'Are you sure you want to delete this profile?');
    if (confirmed) {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.DeleteProfile) {
            await window.go.main.App.DeleteProfile(id);
        } else {
            await apiPost('/api/profiles/delete', { id });
        }
        await refreshConfigAndProfiles();
        showToast('Profile deleted', 'success');
    }
}

async function openProfileModal(profToEdit = null) {
    profileModalEl.classList.remove('hidden');
    presetButtonsEl.innerHTML = '';

    const modalTitle = document.getElementById('modal-title');
    if (profToEdit) {
        editingProfileId = profToEdit.id;
        if (modalTitle) modalTitle.textContent = 'Edit Profile';
        profNameInput.value = profToEdit.name || '';
        profCmdInput.value = profToEdit.command || '';
        profArgsInput.value = (profToEdit.args || []).join(',');
        profDirInput.value = profToEdit.workDir || '';
    } else {
        editingProfileId = null;
        if (modalTitle) modalTitle.textContent = 'Create New Profile';
        profNameInput.value = '';
        profCmdInput.value = '';
        profArgsInput.value = '';
        profDirInput.value = '';
    }

    if (!detectedPresets || detectedPresets.length === 0) {
        try {
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetDetectedProfiles) {
                detectedPresets = await window.go.main.App.GetDetectedProfiles();
            }
        } catch (e) {
            console.error('Failed to get detected presets:', e);
        }
    }

    if (detectedPresets && detectedPresets.length > 0) {
        detectedPresets.forEach(preset => {
            const btn = document.createElement('button');
            btn.className = 'tag-btn';
            btn.textContent = preset.name;
            btn.type = 'button';
            btn.addEventListener('click', () => {
                profNameInput.value = preset.name;
                profCmdInput.value = preset.command;
                profArgsInput.value = (preset.args || []).join(',');
            });
            presetButtonsEl.appendChild(btn);
        });
    } else {
        const span = document.createElement('span');
        span.style.color = 'var(--text-muted)';
        span.style.fontSize = '0.85rem';
        span.textContent = 'No presets detected automatically';
        presetButtonsEl.appendChild(span);
    }
}

function closeProfileModal() {
    editingProfileId = null;
    profileModalEl.classList.add('hidden');
}

async function saveProfileFromModal() {
    let name = profNameInput.value.trim();
    const command = profCmdInput.value.trim();
    const argsStr = profArgsInput.value.trim();
    const workDir = profDirInput.value.trim();

    if (name.length > 256) {
        name = name.slice(0, 256).trim();
    }

    if (!name || !command) {
        showToast('Please specify name and command', 'error');
        return;
    }

    const args = argsStr ? argsStr.split(',').map(s => s.trim()).filter(Boolean) : [];
    const isEdit = !!editingProfileId;
    const prof = { 
        id: editingProfileId || '',
        name, 
        command, 
        args, 
        workDir 
    };

    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.SaveProfile) {
            await window.go.main.App.SaveProfile(prof);
        } else {
            await apiPost('/api/profiles', prof);
        }
        await refreshConfigAndProfiles();
        showToast(isEdit ? 'Profile updated' : 'Profile created', 'success');
        closeProfileModal();
    } catch (e) {
        showToast(`Failed to save profile: ${e.message || e}`, 'error');
    }
}

const DEFAULT_SSH_CLIENT_PATH = 'C:\\Windows\\System32\\OpenSSH\\ssh.exe';

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
    if (!sshConfig || !sshClientPathInput) return;
    sshConfig.clientPath = sshClientPathInput.value.trim() || DEFAULT_SSH_CLIENT_PATH;
    const saved = await persistSSHConfig();
    if (saved) {
        showToast('SSH client path saved', 'success');
    }
}

async function openSSHManager(sessionId, paneId) {
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
    if (sshClientPathInput) {
        sshClientPathInput.value = (sshConfig && sshConfig.clientPath) || DEFAULT_SSH_CLIENT_PATH;
    }
    renderSSHAddressList();
    sshModalEl.classList.remove('hidden');
}

function closeSSHManager() {
    sshModalEl.classList.add('hidden');
    sshPaneId = null;
}

function renderSSHAddressList() {
    sshAddressListEl.innerHTML = '';
    const list = (sshConfig && sshConfig.addresses) || [];
    if (list.length === 0) {
        const empty = document.createElement('div');
        empty.className = 'ssh-empty';
        empty.textContent = 'No addresses saved yet. Click "Add Address" to create one.';
        sshAddressListEl.appendChild(empty);
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
        sshAddressListEl.appendChild(item);
    });
}

function openSSHAddressModal(addrToEdit) {
    editingSshAddressId = addrToEdit ? addrToEdit.id : null;
    const titleEl = document.getElementById('ssh-address-modal-title');
    if (titleEl) titleEl.textContent = addrToEdit ? 'Edit Address' : 'Add Address';
    sshAddrNameInput.value = addrToEdit ? (addrToEdit.name || '') : '';
    sshAddrDescInput.value = addrToEdit ? (addrToEdit.description || '') : '';
    sshAddrHostInput.value = addrToEdit ? (addrToEdit.host || '') : '';
    sshAddrUserInput.value = addrToEdit ? (addrToEdit.user || '') : '';
    sshAddressModalEl.classList.remove('hidden');
    setTimeout(() => sshAddrNameInput.focus(), 50);
}

function closeSSHAddressModal() {
    editingSshAddressId = null;
    sshAddressModalEl.classList.add('hidden');
}

async function saveSSHAddressFromModal() {
    const name = sshAddrNameInput.value.trim();
    const host = sshAddrHostInput.value.trim();
    if (!name || !host) {
        showToast('Please specify name and host address', 'error');
        return;
    }

    const addr = {
        id: editingSshAddressId || `ssh_addr_${Date.now()}`,
        name,
        description: sshAddrDescInput.value.trim(),
        host,
        user: sshAddrUserInput.value.trim()
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
    const paneEntry = activePanesMap.get(sshPaneId);
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
        sshPassTitleEl.textContent = title;
        sshPassMessageEl.textContent = message;
        sshPassInput.value = '';
        sshPassConfirmInput.value = '';
        sshPassConfirmGroupEl.classList.toggle('hidden', mode !== 'export');
        sshPassModalEl.classList.remove('hidden');
        setTimeout(() => sshPassInput.focus(), 50);
    });
}

function cancelSSHPasswordModal() {
    sshPassModalEl.classList.add('hidden');
    if (sshPassResolve) {
        sshPassResolve(null);
        sshPassResolve = null;
    }
}

function confirmSSHPasswordModal() {
    const pwd = sshPassInput.value;
    if (sshPassMode === 'export') {
        if (!pwd) {
            showToast('Please enter a password', 'error');
            return;
        }
        if (pwd !== sshPassConfirmInput.value) {
            showToast('Passwords do not match', 'error');
            return;
        }
    } else if (!pwd) {
        showToast('Please enter the password', 'error');
        return;
    }
    sshPassModalEl.classList.add('hidden');
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
        if (sshClientPathInput) {
            sshClientPathInput.value = (sshConfig && sshConfig.clientPath) || DEFAULT_SSH_CLIENT_PATH;
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

// Git Status Collapsible Floating Panel & Resizer
function initGitPanelResizer() {    const resizer = document.getElementById('git-panel-resizer');
    if (!resizer || !gitPanelEl) return;

    let isResizing = false;

    resizer.addEventListener('mousedown', (e) => {
        isResizing = true;
        resizer.classList.add('dragging');
        document.body.style.cursor = 'ew-resize';
        document.body.style.userSelect = 'none';
    });

    document.addEventListener('mousemove', (e) => {
        if (!isResizing) return;
        const newWidth = window.innerWidth - e.clientX;
        if (newWidth >= 250 && newWidth <= 800) {
            gitPanelEl.style.width = `${newWidth}px`;
        }
    });

    document.addEventListener('mouseup', () => {
        if (isResizing) {
            isResizing = false;
            resizer.classList.remove('dragging');
            document.body.style.cursor = '';
            document.body.style.userSelect = '';
        }
    });
}

function updateGitButtonBadge(count) {
    const badgeEl = document.getElementById('git-btn-badge');
    if (!badgeEl) return;
    if (count > 0) {
        badgeEl.textContent = count > 99 ? '99+' : count;
        badgeEl.classList.remove('hidden');
    } else {
        badgeEl.classList.add('hidden');
    }
}

async function updateGitStatus() {
    if (!activePaneId) {
        updateGitButtonBadge(0);
        return;
    }

    const paneInfo = activePanesMap.get(activePaneId);
    const workDir = paneInfo ? paneInfo.workDir : '';

    let res = null;
    if (window.go && window.go.main && window.go.main.App) {
        res = await window.go.main.App.GetGitStatus(workDir);
    } else {
        res = await apiGet(`/api/git/status?dir=${encodeURIComponent(workDir)}`);
    }

    if (!res || !res.isGitRepo) {
        updateGitButtonBadge(0);
        if (!gitPanelEl.classList.contains('hidden')) {
            gitBranchBadgeEl.textContent = 'Not a git repo';
            gitChangesContainerEl.innerHTML = '<div class="git-empty">No git repository detected in active pane.</div>';
        }
        return;
    }

    const changeCount = (res.changes && res.changes.length) ? res.changes.length : 0;
    updateGitButtonBadge(changeCount);

    if (gitPanelEl.classList.contains('hidden')) return;

    gitBranchBadgeEl.textContent = res.branch || 'main';
    gitChangesContainerEl.innerHTML = '';

    if (!res.changes || res.changes.length === 0) {
        gitChangesContainerEl.innerHTML = '<div class="git-empty">Working tree clean</div>';
        return;
    }

    // Group changes by directory (excluding filename)
    const groups = {};

    res.changes.forEach(ch => {
        const lastSlash = ch.path.lastIndexOf('/');
        let dir = '.';
        let filename = ch.path;
        if (lastSlash !== -1) {
            dir = ch.path.substring(0, lastSlash);
            filename = ch.path.substring(lastSlash + 1);
        }

        if (!groups[dir]) {
            groups[dir] = [];
        }
        groups[dir].push({
            status: ch.status,
            filename: filename,
            fullPath: ch.path
        });
    });

    Object.keys(groups).sort().forEach(dir => {
        const groupEl = document.createElement('div');
        groupEl.className = 'git-group';

        const header = document.createElement('div');
        header.className = 'git-group-header';
        header.innerHTML = `📂 <span class="git-group-dir">${dir}</span>`;
        groupEl.appendChild(header);

        const itemsBox = document.createElement('div');
        itemsBox.className = 'git-group-items';

        groups[dir].forEach(item => {
            const itemEl = document.createElement('div');
            itemEl.className = 'git-file-item';
            itemEl.title = item.fullPath;
            itemEl.innerHTML = `<span class="status-tag ${item.status}">${item.status}</span> <span class="git-file-name">${item.filename}</span>`;
            itemsBox.appendChild(itemEl);
        });

        groupEl.appendChild(itemsBox);
        gitChangesContainerEl.appendChild(groupEl);
    });
}
