import '@xterm/xterm/css/xterm.css';
import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';

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
        reflowAllPanes(true);
        setTimeout(() => reflowAllPanes(true), 80);
    });

    document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible') {
            reflowAllPanes(true);
            setTimeout(() => reflowAllPanes(true), 80);
        }
    });

    window.addEventListener('resize', () => {
        reflowAllPanes(true);
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

    if (activeSessionTitleEl) {
        activeSessionTitleEl.addEventListener('click', () => startEditingSessionTitle());
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
        renderSessionList();
        if (sessions.length > 0 && !activeSession) {
            attachToSession(sessions[0].id);
        }
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

    if (!activeSession) return;

    const newName = activeSessionTitleEl.textContent.trim();
    if (!commit || !newName || newName === activeSession.name) {
        activeSessionTitleEl.textContent = activeSession.name;
        return;
    }

    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.RenameSession) {
            await window.go.main.App.RenameSession(activeSession.id, newName);
        } else {
            await apiPost('/api/sessions/rename', { sessionId: activeSession.id, newName });
        }
        await refreshSessions();
        showToast(`Session renamed to "${newName}"`, 'info');
    } catch (e) {
        activeSessionTitleEl.textContent = activeSession.name;
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

function cleanupInactivePanes(sess) {
    if (!sess || !sess.panes) return;
    activePanesMap.forEach((entry, paneId) => {
        if (!sess.panes[paneId]) {
            // Pane was closed remotely or removed from layout
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
    if (activeSessionTitleEl) activeSessionTitleEl.textContent = 'No Active Session';
    if (btnRenameSessionEl) btnRenameSessionEl.classList.add('hidden');
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
    activeSessionTitleEl.textContent = sess.name;
    if (btnRenameSessionEl) btnRenameSessionEl.classList.remove('hidden');
    renderSessionList();

    // Cleanup any deleted/inactive panes from activePanesMap
    cleanupInactivePanes(sess);

    // Render Terminal Grid layout
    terminalWorkspaceEl.innerHTML = '';
    emptyStateEl.style.display = 'none';

    const layoutContainer = renderLayoutTree(sess.layout, sess);
    terminalWorkspaceEl.appendChild(layoutContainer);

    // Set focus to active pane
    if (sess.activePaneId && activePanesMap.has(sess.activePaneId)) {
        setActivePane(sess.activePaneId, false);
    }
    
    // Immediate and staged reflow passes to guarantee layout calculation, screen repaint, and ConPTY resize sync
    reflowAllPanes(true);
    setTimeout(() => reflowAllPanes(true), 60);
    setTimeout(() => reflowAllPanes(true), 150);
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
            reflowAllPanes(true);
        });

        const paneData = sess.panes[node.id];
        if (paneData) {
            initXtermPane(paneEl, paneData);
        }

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

    const updateSessionsExpandState = () => {
        if (sectionProfiles && sectionSessions) {
            if (sectionProfiles.classList.contains('collapsed')) {
                sectionSessions.classList.add('expand-fill');
            } else {
                sectionSessions.classList.remove('expand-fill');
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
    updateSessionsExpandState();

    if (headerSessions && sectionSessions) {
        headerSessions.addEventListener('click', () => {
            sectionSessions.classList.toggle('collapsed');
            localStorage.setItem('pmux_sessions_collapsed', sectionSessions.classList.contains('collapsed'));
        });
    }

    if (headerProfiles && sectionProfiles) {
        headerProfiles.addEventListener('click', () => {
            sectionProfiles.classList.toggle('collapsed');
            localStorage.setItem('pmux_profiles_collapsed', sectionProfiles.classList.contains('collapsed'));
            updateSessionsExpandState();
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
        setTimeout(() => {
            try {
                existing.fitAddon.fit();
                if (existing.term) {
                    existing.term.refresh(0, existing.term.rows - 1);
                }
                if (existing.ws && existing.ws.readyState === WebSocket.OPEN && existing.term && existing.term.cols >= 10 && existing.term.rows >= 3) {
                    existing.ws.send(JSON.stringify({ type: 'resize', cols: existing.term.cols, rows: existing.term.rows }));
                }
            } catch(e) {}
        }, 30);
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

    const connectPaneWS = () => {
        const wsUrl = `ws://127.0.0.1:${serverPort}/ws/pane/${paneData.id}`;
        const ws = new WebSocket(wsUrl);

        if (activePanesMap.has(paneData.id)) {
            activePanesMap.get(paneData.id).ws = ws;
        }

        ws.onopen = () => {
            console.log(`WebSocket connected to pane ${paneData.id} at ${wsUrl}`);
            if (isReconnecting) {
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

        ws.onmessage = async (event) => {
            let text = '';
            if (typeof event.data === 'string') {
                text = event.data;
            } else if (event.data instanceof Blob) {
                text = await event.data.text();
            }
            if (text) {
                term.write(text);
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

    term.onData((data) => {
        const paneEntry = activePanesMap.get(paneData.id);
        const currentWS = paneEntry ? paneEntry.ws : ws;
        if (currentWS && currentWS.readyState === WebSocket.OPEN) {
            currentWS.send(JSON.stringify({ type: 'input', data: data }));
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

            if (isCtrlV) {
                if (event.shiftKey) {
                    // Ctrl + Shift + V: Traditional Literal Next (^V / \x16)
                    return true;
                } else {
                    // Ctrl + V: Prevent xterm from sending \x16 to backend,
                    // allowing browser native 'paste' event to handle clipboard paste exactly once.
                    return false;
                }
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
        try {
            p.fitAddon.fit();
            p.term.refresh(0, p.term.rows - 1);
            if (p.ws && p.ws.readyState === WebSocket.OPEN && p.term.cols >= 10 && p.term.rows >= 3) {
                p.ws.send(JSON.stringify({ type: 'resize', cols: p.term.cols, rows: p.term.rows }));
            }
        } catch(e) {}
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

function reflowAllPanes(forceSendResize = false) {
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
                        if (forceSendResize && ws && ws.readyState === WebSocket.OPEN && term.cols >= 10 && term.rows >= 3) {
                            ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
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
    const name = profNameInput.value.trim();
    const command = profCmdInput.value.trim();
    const argsStr = profArgsInput.value.trim();
    const workDir = profDirInput.value.trim();

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

// Git Status Collapsible Floating Panel & Resizer
function initGitPanelResizer() {
    const resizer = document.getElementById('git-panel-resizer');
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
