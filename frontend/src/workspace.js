import '@xterm/xterm/css/xterm.css';
import { Terminal } from '@xterm/xterm';
import { WebglAddon } from '@xterm/addon-webgl';
import sshWhiteIcon from './assets/images/ssh-white.svg';
import { state } from './state.js';
import { dom, addClick } from './dom.js';
import { apiGet, apiPost } from './api.js';
import { showToast, showConfirm, fallbackCopyTextToClipboard } from './ui.js';
import { reflowAllPanes, applyViewportClip, sendViewportResize } from './viewport.js';
import { openSSHManager } from './ssh.js';
import { closeTodoModal } from './todo.js';
import { updateGitStatus, openWorkFolder } from './git.js';

// --- Session Management ---

export async function refreshSessions() {
    try {
        // Query central daemon server REST API first to guarantee 0.01s real-time multi-client sync
        try {
            state.sessions = await apiGet('/api/sessions');
        } catch (e) {
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetSessions) {
                state.sessions = await window.go.main.App.GetSessions();
            } else {
                throw e;
            }
        }

        if (state.activeSession) {
            const updatedActiveSess = state.sessions.find(s => s.id === state.activeSession.id);
            if (updatedActiveSess) {
                state.activeSession = updatedActiveSess;
                if (!isEditingSessionTitle) {
                    setSessionTitle(state.activeSession.name);
                }
            } else if (state.sessions.length === 0) {
                showEmptyState();
            }
        } else if (state.sessions.length > 0) {
            attachToSession(state.sessions[0].id);
        }

        renderSessionList();
    } catch (e) {
        console.error('Failed to refresh sessions:', e);
    }
}

export function renderSessionList() {
    dom.sessionListEl.innerHTML = '';
    if (!state.sessions || state.sessions.length === 0) {
        const emptyLi = document.createElement('li');
        emptyLi.textContent = 'No sessions';
        emptyLi.style.color = 'var(--text-muted)';
        emptyLi.style.cursor = 'default';
        dom.sessionListEl.appendChild(emptyLi);
        return;
    }

    state.sessions.forEach(sess => {
        const li = document.createElement('li');
        li.title = sess.name;
        li.innerHTML = `<span>💻 ${sess.name}</span><button class="icon-btn-small del-btn" title="Close Session">✖</button>`;
        if (state.activeSession && state.activeSession.id === sess.id) {
            li.classList.add('active');
        }
        li.addEventListener('click', () => attachToSession(sess.id));
        li.querySelector('.del-btn').addEventListener('click', async (e) => {
            e.stopPropagation();
            await closeSession(sess.id, sess.name);
        });
        dom.sessionListEl.appendChild(li);
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
        const sess = state.sessions.find(s => s.id === sessionId);
        if (sess && sess.panes) {
            Object.keys(sess.panes).forEach(paneId => {
                if (state.activePanesMap.has(paneId)) {
                    const p = state.activePanesMap.get(paneId);
                    try { p.ws.close(); } catch(e){}
                    state.activePanesMap.delete(paneId);
                }
            });
        }

        if (state.activeSession && state.activeSession.id === sessionId) {
            state.activeSession = null;
            state.activePaneId = null;
            dom.terminalWorkspaceEl.innerHTML = '';
            dom.emptyStateEl.style.display = 'flex';
            dom.activeSessionTitleEl.textContent = 'No Active Session';
            if (dom.btnRenameSessionEl) dom.btnRenameSessionEl.classList.add('hidden');
            if (dom.btnOpenWorkFolderEl) dom.btnOpenWorkFolderEl.classList.add('hidden');
            if (dom.btnRefreshSessionPanesEl) dom.btnRefreshSessionPanesEl.classList.add('hidden');
        }

        await refreshSessions();
        showToast(`Closed session: ${sessionName}`, 'success');
    } catch (e) {
        showToast(`Failed to close session: ${e.message || e}`, 'error');
    }
}

function generateSessionName(profName) {
    const baseName = profName;
    const existingNames = state.sessions.map(s => s.name);

    if (!existingNames.includes(baseName)) {
        return baseName;
    }

    let num = 2;
    while (existingNames.includes(`${baseName} #${num}`)) {
        num++;
    }
    return `${baseName} #${num}`;
}

export function setSessionTitle(name) {
    if (!dom.activeSessionTitleEl) return;
    dom.activeSessionTitleEl.textContent = name;
    dom.activeSessionTitleEl.title = name;
    dom.activeSessionTitleEl.scrollLeft = 0;
}

let isEditingSessionTitle = false;

export function startEditingSessionTitle() {
    if (!state.activeSession || isEditingSessionTitle) return;
    isEditingSessionTitle = true;
    dom.activeSessionTitleEl.contentEditable = "true";
    dom.activeSessionTitleEl.focus();

    try {
        const range = document.createRange();
        range.selectNodeContents(dom.activeSessionTitleEl);
        const sel = window.getSelection();
        sel.removeAllRanges();
        sel.addRange(range);
    } catch (e) {}
}

export async function finishEditingSessionTitle(commit = true) {
    if (!isEditingSessionTitle) return;
    isEditingSessionTitle = false;
    dom.activeSessionTitleEl.contentEditable = "false";
    dom.activeSessionTitleEl.scrollLeft = 0;

    if (!state.activeSession) return;

    const rawText = dom.activeSessionTitleEl.innerText || dom.activeSessionTitleEl.textContent || '';
    let newName = rawText.replace(/\u00a0/g, ' ').replace(/[\r\n]+/g, ' ').trim();
    if (newName.length > 256) {
        newName = newName.slice(0, 256).trim();
    }
    if (!commit || !newName || newName === state.activeSession.name) {
        setSessionTitle(state.activeSession.name);
        return;
    }

    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.RenameSession) {
            await window.go.main.App.RenameSession(state.activeSession.id, newName);
        } else {
            await apiPost('/api/sessions/rename', { sessionId: state.activeSession.id, newName });
        }
        state.activeSession.name = newName;
        setSessionTitle(newName);
        await refreshSessions();
        showToast(`Session renamed to "${newName}"`, 'info');
    } catch (e) {
        setSessionTitle(state.activeSession ? state.activeSession.name : 'No Active Session');
        showToast('Failed to rename session: ' + (e.message || e), 'error');
    }
}

export async function createSessionFromProfile(prof) {
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
    state.sessions.forEach(s => {
        if (s && s.panes) {
            Object.keys(s.panes).forEach(id => validPaneIds.add(id));
        }
    });

    state.activePanesMap.forEach((entry, paneId) => {
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
            state.activePanesMap.delete(paneId);
        }
    });
}

export function showEmptyState() {
    state.activeSession = null;
    state.activePaneId = null;
    closeTodoModal();
    setSessionTitle('No Active Session');
    if (dom.btnRenameSessionEl) dom.btnRenameSessionEl.classList.add('hidden');
    if (dom.btnOpenWorkFolderEl) dom.btnOpenWorkFolderEl.classList.add('hidden');
    if (dom.btnRefreshSessionPanesEl) dom.btnRefreshSessionPanesEl.classList.add('hidden');
    if (dom.terminalWorkspaceEl) dom.terminalWorkspaceEl.innerHTML = '';
    if (dom.emptyStateEl) dom.emptyStateEl.style.display = 'flex';
    renderSessionList();
}

export async function attachToSession(sessionId) {
    const sess = state.sessions.find(s => s.id === sessionId);
    if (!sess) {
        if (state.sessions.length > 0) {
            attachToSession(state.sessions[0].id);
        } else {
            showEmptyState();
        }
        return;
    }

    state.activeSession = sess;
    closeTodoModal();
    setSessionTitle(sess.name);
    if (dom.btnRenameSessionEl) dom.btnRenameSessionEl.classList.remove('hidden');
    if (dom.btnOpenWorkFolderEl) dom.btnOpenWorkFolderEl.classList.remove('hidden');
    if (dom.btnRefreshSessionPanesEl) dom.btnRefreshSessionPanesEl.classList.remove('hidden');
    renderSessionList();

    // Cleanup any deleted/orphan panes from activePanesMap (across all sessions)
    cleanupOrphanPanes();

    // Render Terminal Grid layout
    dom.terminalWorkspaceEl.innerHTML = '';
    dom.emptyStateEl.style.display = 'none';

    const layoutContainer = renderLayoutTree(sess.layout, sess);
    dom.terminalWorkspaceEl.appendChild(layoutContainer);

    // Set focus to active pane
    if (sess.activePaneId && state.activePanesMap.has(sess.activePaneId)) {
        setActivePane(sess.activePaneId, false);
    }
    
    // Coordinated reflow pass to calculate layout and synchronize ConPTY geometry
    requestAnimationFrame(() => {
        reflowAllPanes(true, false);
    });
}

// --- Layout Tree & Splitters ---

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
    const btnAddFolder = document.getElementById('btn-add-folder');

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

    // Prevent Add Folder button click from triggering section toggle
    if (btnAddFolder) {
        btnAddFolder.addEventListener('click', (e) => {
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

// --- Terminal Panes ---

function initXtermPane(containerEl, paneData) {
    if (state.activePanesMap.has(paneData.id)) {
        // Re-attach existing term element
        const existing = state.activePanesMap.get(paneData.id);
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

    const viewportBadge = document.createElement('div');
    viewportBadge.className = 'viewport-badge';
    termBox.appendChild(viewportBadge);

    const term = new Terminal({
        fontFamily: '"Cascadia Code", "Fira Code", Consolas, monospace',
        fontSize: 14,
        theme: {
            background: '#0d0e11',
            foreground: '#abb2bf',
            cursor: '#528bff'
        },
        cursorBlink: true,
        alternateScroll: true,
        allowProposedApi: true
    });

    term.open(termBox);

    try {
        const webglAddon = new WebglAddon();
        webglAddon.onContextLoss(() => {
            webglAddon.dispose();
        });
        term.loadAddon(webglAddon);
    } catch (e) {
        // WebGL fallback to DOM renderer
    }

    setTimeout(() => {
        try {
            term.focus();
        } catch (e) {}
    }, 100);

    // WebSocket connection to ConPTY stream with 3-retry 1s-delay management
    let retryCount = 0;
    let hasEverConnected = false;
    let isReconnecting = false;
    let isWritingServerOutput = false;
    let isReplayingHistory = true;

    // Detect terminal internal auto-responses (e.g. OSC 10/11/4 color queries, DSR cursor reports, DECRQM mode reports)
    const isTerminalQueryResponse = (str) => {
        if (!str || typeof str !== 'string') return false;
        // OSC 10/11/4 color query responses: \x1b]10;rgb:..., \x1b]11;rgb:..., \x1b]4;0;rgb:...
        if (str.startsWith('\x1b]10;') || str.startsWith('\x1b]11;') || str.startsWith('\x1b]4;') || str.includes(';rgb:')) {
            return true;
        }
        // DSR cursor position reports: \x1b[1;1R or \x1b[row;colR
        if (/^\x1b\[\d+;\d+R$/.test(str)) {
            return true;
        }
        // DECRQM mode reports: \x1b[?1016;2$y, \x1b[?2004;2$y, etc.
        if (/^\x1b\[\?\d+;\d+\$y$/.test(str)) {
            return true;
        }
        // Device Attributes: \x1b[?1;2c, \x1b[>0;...c
        if (/^\x1b\[(\?|>)\d+.*c$/.test(str)) {
            return true;
        }
        return false;
    };

    const connectPaneWS = () => {
        const wsUrl = `ws://127.0.0.1:${state.serverPort}/ws/pane/${paneData.id}`;
        const ws = new WebSocket(wsUrl);

        if (state.activePanesMap.has(paneData.id)) {
            state.activePanesMap.get(paneData.id).ws = ws;
        }

        ws.onopen = () => {
            console.log(`WebSocket connected to pane ${paneData.id} at ${wsUrl}`);
            isReplayingHistory = true;
            setTimeout(() => { isReplayingHistory = false; }, 600);

            if (isReconnecting) {
                showToast('Terminal connection restored', 'success');
                isReconnecting = false;
            }
            hasEverConnected = true;
            retryCount = 0; // Reset retry count on successful connection
            setTimeout(() => {
                try {
                    term.focus();
                    // Report this client's own viewport size so the server can
                    // compute the pane's canonical (largest) size. The terminal
                    // buffer itself is sized by the pane-size control message.
                    const paneEntry = state.activePanesMap.get(paneData.id) || { term, ws, element: termBox };
                    sendViewportResize(paneEntry, true);
                } catch (e) {}
            }, 100);
        };

        ws.binaryType = 'arraybuffer';

        const safeTermWrite = (data) => {
            isWritingServerOutput = true;
            try {
                term.write(data, () => {
                    isWritingServerOutput = false;
                });
            } catch (e) {
                isWritingServerOutput = false;
            }
        };

        ws.onmessage = (event) => {
            if (typeof event.data === 'string') {
                // Text frames are JSON control messages (e.g. pane-size);
                // terminal data is always sent as binary frames.
                try {
                    const msg = JSON.parse(event.data);
                    if (msg && msg.type === 'pane-size') {
                        if (msg.cols >= 10 && msg.rows >= 3 && (msg.cols !== term.cols || msg.rows !== term.rows)) {
                            term.resize(msg.cols, msg.rows);
                        }
                        applyViewportClip(termBox, term);
                        // Re-report this client's viewport now that the
                        // canonical size is known. A freshly attached client's
                        // earlier onopen report may have been skipped because
                        // the layout/renderer was not ready yet — without this,
                        // the server would never learn the client's real size
                        // (canonical stays stale until a manual refresh).
                        sendViewportResize(state.activePanesMap.get(paneData.id) || { term, ws, element: termBox }, true);
                        return;
                    }
                } catch (e) {}
                // Fallback: raw text terminal data
                safeTermWrite(event.data);
            } else if (event.data instanceof ArrayBuffer) {
                safeTermWrite(new Uint8Array(event.data));
            } else if (event.data instanceof Blob) {
                event.data.text().then(text => {
                    if (text) safeTermWrite(text);
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
            if (!state.activePanesMap.has(paneData.id)) {
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
                    if (state.activePanesMap.has(paneData.id)) {
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
        const paneEntry = state.activePanesMap.get(paneData.id);
        const currentWS = paneEntry ? paneEntry.ws : ws;
        if (currentWS && currentWS.readyState === WebSocket.OPEN) {
            currentWS.send(JSON.stringify({ type: 'input', data: toSend }));
        }
    };

    term.onData((data) => {
        // Prevent xterm.js auto-replies (e.g. OSC color responses, cursor position reports)
        // generated during history replay or server output rendering from echoing back to the shell stdin.
        if (isWritingServerOutput || isReplayingHistory) {
            if (isTerminalQueryResponse(data)) {
                return;
            }
        }
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
                const paneEntry = state.activePanesMap.get(paneData.id);
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

    // Buffer size changes only via the server's pane-size control message
    // (never by the container), so onResize just refreshes the viewport clip.
    term.onResize(() => {
        applyViewportClip(termBox, term);
    });

    state.activePanesMap.set(paneData.id, {
        term,
        ws,
        element: termBox,
        viewportCols: 0,
        viewportRows: 0,
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
        
        if (state.activePanesMap.has(paneId)) {
            const paneObj = state.activePanesMap.get(paneId);
            if (paneObj.ws) paneObj.ws.close();
            state.activePanesMap.delete(paneId);
        }

        await refreshSessions();
        if (state.activeSession) {
            attachToSession(state.activeSession.id);
        } else {
            dom.terminalWorkspaceEl.innerHTML = '';
            dom.emptyStateEl.style.display = 'flex';
        }
        showToast('Pane closed', 'info');
    } catch (e) {
        showToast('Failed to close pane: ' + (e.message || e), 'error');
    }
}

export function setActivePane(paneId, notifyServer = true) {
    state.activePaneId = paneId;
    document.querySelectorAll('.pane-element').forEach(el => el.classList.remove('active'));
    const box = document.getElementById(`pane-box-${paneId}`);
    if (box) box.classList.add('active');

    if (state.activePanesMap.has(paneId)) {
        const p = state.activePanesMap.get(paneId);
        p.term.focus();
    }
    updateGitStatus();

    if (notifyServer && state.activeSession) {
        apiPost('/api/sessions/active-pane', { sessionId: state.activeSession.id, paneId }).catch(() => {});
    }
}

export async function splitCurrentPane(direction) {
    if (!state.activeSession || !state.activePaneId) return;

    const parentPaneData = state.activeSession.panes ? state.activeSession.panes[state.activePaneId] : null;
    const currentPane = state.activePanesMap.get(state.activePaneId);
    const fallbackProf = state.profiles[0] || state.detectedPresets[0] || { command: 'cmd.exe', args: [] };

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
        sessionId: state.activeSession.id,
        parentPaneId: state.activePaneId,
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
            attachToSession(state.activeSession.id);
            setActivePane(newPane.id);
            reflowAllPanes();
            showToast('Pane split successfully', 'success');
        }
    } catch (e) {
        showToast('Failed to split pane: ' + (e.message || e), 'error');
    }
}

export function initWorkspaceEvents() {
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
        setTimeout(() => reflowAllPanes(true, false), 60);
    });

    document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible') {
            setTimeout(() => reflowAllPanes(true, false), 100);
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

    addClick('btn-split-v', () => {
        if (state.activeSession && state.activePaneId) {
            splitCurrentPane('vertical');
        } else {
            showToast('No active pane to split', 'error');
        }
    });

    addClick('btn-split-h', () => {
        if (state.activeSession && state.activePaneId) {
            splitCurrentPane('horizontal');
        } else {
            showToast('No active pane to split', 'error');
        }
    });

    addClick('btn-rename-session', () => startEditingSessionTitle());
    addClick('btn-open-work-folder', () => openWorkFolder());
    addClick('btn-refresh-session-panes', () => {
        if (!state.activeSession) return;
        reflowAllPanes(true, true);
        setTimeout(() => reflowAllPanes(true, true), 60);
        showToast('Session panes refreshed', 'info');
    });

    if (dom.activeSessionTitleEl) {
        dom.activeSessionTitleEl.addEventListener('click', () => startEditingSessionTitle());
        dom.activeSessionTitleEl.addEventListener('input', () => {
            const raw = dom.activeSessionTitleEl.innerText || dom.activeSessionTitleEl.textContent || '';
            if (raw.length > 256) {
                const trimmed = raw.slice(0, 256);
                dom.activeSessionTitleEl.textContent = trimmed;
                try {
                    const range = document.createRange();
                    range.selectNodeContents(dom.activeSessionTitleEl);
                    range.collapse(false);
                    const sel = window.getSelection();
                    sel.removeAllRanges();
                    sel.addRange(range);
                } catch (e) {}
            }
        });
        dom.activeSessionTitleEl.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                dom.activeSessionTitleEl.blur();
            } else if (e.key === 'Escape') {
                finishEditingSessionTitle(false);
            }
        });
        dom.activeSessionTitleEl.addEventListener('blur', () => finishEditingSessionTitle(true));
    }

    addClick('btn-refresh-sessions', refreshSessions);
}
