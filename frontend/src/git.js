import { state, getActiveWorkDir } from './state.js';
import { dom, addClick } from './dom.js';
import { apiGet, apiPost, isWails } from './api.js';
import { showToast } from './ui.js';

let gitPollTimer = null;

export function startGitPollTimer(seconds) {
    if (gitPollTimer) {
        clearInterval(gitPollTimer);
    }
    const sec = parseInt(seconds, 10) || 3;
    gitPollTimer = setInterval(updateGitStatus, sec * 1000);
}

export function applyGitPollInterval(interval) {
    const sec = parseInt(interval, 10);
    if (!sec || sec < 1 || sec === state.currentGitPollInterval) return;
    state.currentGitPollInterval = sec;
    const pollSelectEl = document.getElementById('git-poll-interval-select');
    if (pollSelectEl) pollSelectEl.value = String(sec);
    startGitPollTimer(sec);
}

// Git Status Collapsible Floating Panel & Resizer
function initGitPanelResizer() {
    const resizer = document.getElementById('git-panel-resizer');
    if (!resizer || !dom.gitPanelEl) return;

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
        if (newWidth >= 250 && newWidth <= 1000) {
            dom.gitPanelEl.style.width = `${newWidth}px`;
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

async function callOpenWorkFolder(workDir) {
    if (isWails()) return await window.go.main.App.OpenWorkFolder(workDir);
    return await apiPost('/api/sessions/open-work-folder', { workDir });
}

export async function openWorkFolder() {
    if (!state.activeSession) return;
    const workDir = getActiveWorkDir();
    if (!workDir) {
        showToast('No work folder for the active pane', 'error');
        return;
    }
    try {
        await callOpenWorkFolder(workDir);
        showToast(`Opened work folder: ${workDir}`, 'success');
    } catch (e) {
        console.error('Failed to open work folder:', e);
        showToast(`Failed to open work folder: ${workDir}`, 'error');
    }
}

let selectedDiffPath = null;
let selectedCommitHash = null;

export async function updateGitStatus() {
    const workDir = getActiveWorkDir();
    if (!workDir) {
        updateGitButtonBadge(0);
        if (!dom.gitPanelEl.classList.contains('hidden')) {
            renderGitEmpty('No active pane.');
        }
        return;
    }

    let res = null;
    try {
        res = await callGitStatus(workDir);
    } catch (e) {
        console.error('git status failed:', e);
        updateGitButtonBadge(0);
        return;
    }

    if (!res || !res.isGitRepo) {
        updateGitButtonBadge(0);
        if (!dom.gitPanelEl.classList.contains('hidden')) {
            renderGitEmpty('No git repository detected in active pane.');
        }
        return;
    }

    const changeCount = (res.changes && res.changes.length) ? res.changes.length : 0;
    updateGitButtonBadge(changeCount);

    if (dom.gitPanelEl.classList.contains('hidden')) return;

    dom.gitBranchBadgeEl.textContent = res.branch || 'main';
    dom.gitBranchBadgeEl.title = res.root || '';

    const stagedCount = res.changes ? res.changes.filter(ch => ch.staged).length : 0;
    if (dom.gitStagedCountEl) dom.gitStagedCountEl.textContent = `${stagedCount} staged`;
    if (dom.gitChangesCountEl) dom.gitChangesCountEl.textContent = `${changeCount}`;

    renderGitChanges(workDir, res.changes);

    // Refresh the diff preview for the currently selected file, if it still
    // shows up in the status.
    if (selectedDiffPath) {
        const stillThere = res.changes && res.changes.some(ch => ch.path === selectedDiffPath);
        if (stillThere) {
            showDiff(workDir, selectedDiffPath);
        } else {
            closeDiffView();
        }
    }

    renderGitBranches(workDir, res.branch);
    renderGitLog(workDir);
}

function renderGitEmpty(msg) {
    dom.gitBranchBadgeEl.textContent = 'Not a git repo';
    dom.gitChangesContainerEl.innerHTML = `<div class="git-empty">${msg}</div>`;
    if (dom.gitLogContainerEl) dom.gitLogContainerEl.innerHTML = '';
    if (dom.gitStagedCountEl) dom.gitStagedCountEl.textContent = '';
    if (dom.gitChangesCountEl) dom.gitChangesCountEl.textContent = '';
}

// --- Git API calls (Wails bindings with HTTP API fallback) ---

async function callGitStatus(workDir) {
    if (isWails()) return await window.go.main.App.GetGitStatus(workDir);
    return await apiGet(`/api/git/status?dir=${encodeURIComponent(workDir)}`);
}

async function callGitPush(workDir) {
    if (isWails()) return await window.go.main.App.GitPush(workDir);
    return await apiPost('/api/git/push', { workDir });
}

async function callGitPull(workDir) {
    if (isWails()) return await window.go.main.App.GitPull(workDir);
    return await apiPost('/api/git/pull', { workDir });
}

async function callGitFetch(workDir) {
    if (isWails()) return await window.go.main.App.GitFetch(workDir);
    return await apiPost('/api/git/fetch', { workDir });
}

async function callGitCommit(workDir, message) {
    if (isWails()) return await window.go.main.App.GitCommit(workDir, message);
    return await apiPost('/api/git/commit', { workDir, message });
}

async function callGitStageAll(workDir) {
    if (isWails()) return await window.go.main.App.GitStageAll(workDir);
    return await apiPost('/api/git/stage-all', { workDir });
}

async function callGitStagePaths(workDir, paths) {
    if (isWails()) return await window.go.main.App.GitStage(workDir, paths);
    return await apiPost('/api/git/stage', { workDir, paths });
}

async function callGitUnstagePaths(workDir, paths) {
    if (isWails()) return await window.go.main.App.GitUnstage(workDir, paths);
    return await apiPost('/api/git/unstage', { workDir, paths });
}

async function callGitCheckout(workDir, branch) {
    if (isWails()) return await window.go.main.App.GitCheckout(workDir, branch);
    return await apiPost('/api/git/checkout', { workDir, branch });
}

async function callGitLog(workDir) {
    if (isWails()) return await window.go.main.App.GetGitLog(workDir, 30);
    return await apiGet(`/api/git/log?dir=${encodeURIComponent(workDir)}&limit=30`);
}

async function callGitBranches(workDir) {
    if (isWails()) return await window.go.main.App.GetGitBranches(workDir);
    return await apiGet(`/api/git/branches?dir=${encodeURIComponent(workDir)}`);
}

async function callGitDiff(workDir, path) {
    if (isWails()) return await window.go.main.App.GetGitDiff(workDir, path);
    return await apiGet(`/api/git/diff?dir=${encodeURIComponent(workDir)}&path=${encodeURIComponent(path)}`);
}

async function callGitShow(workDir, hash) {
    if (isWails()) return await window.go.main.App.GetGitCommitDetail(workDir, hash);
    return await apiGet(`/api/git/show?dir=${encodeURIComponent(workDir)}&hash=${encodeURIComponent(hash)}`);
}

// --- Git actions ---

async function gitRemoteAction(action) {
    const workDir = getActiveWorkDir();
    if (!workDir) return;
    const name = { fetch: 'Fetch', pull: 'Pull', push: 'Push' }[action];
    if (!name) return;
    let res;
    try {
        if (action === 'fetch') res = await callGitFetch(workDir);
        else if (action === 'pull') res = await callGitPull(workDir);
        else res = await callGitPush(workDir);
    } catch (e) {
        showOpOutput({ success: false, error: `${name} failed: ${e}` });
        return;
    }
    showOpOutput(res);
    await updateGitStatus();
}

async function gitCommitAction() {
    const workDir = getActiveWorkDir();
    if (!workDir) return;
    const inputEl = document.getElementById('git-commit-message');
    const message = inputEl ? inputEl.value.trim() : '';
    if (!message) {
        showToast('Enter a commit message first', 'warning');
        if (inputEl) inputEl.focus();
        return;
    }
    const res = await callGitCommit(workDir, message);
    showOpOutput(res);
    if (inputEl) inputEl.value = '';
    await updateGitStatus();
}

async function gitStageAllAction() {
    const workDir = getActiveWorkDir();
    if (!workDir) return;
    const res = await callGitStageAll(workDir);
    showOpOutput(res);
    await updateGitStatus();
}

async function gitStagePath(workDir, path, staged) {
    const res = staged
        ? await callGitUnstagePaths(workDir, [path])
        : await callGitStagePaths(workDir, [path]);
    showOpOutput(res);
    await updateGitStatus();
}

// --- Rendering ---

function renderGitChanges(workDir, changes) {
    dom.gitChangesContainerEl.innerHTML = '';

    if (!changes || changes.length === 0) {
        dom.gitChangesContainerEl.innerHTML = '<div class="git-empty">Working tree clean</div>';
        return;
    }

    // Group changes by directory (excluding filename)
    const groups = {};
    changes.forEach(ch => {
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
        groups[dir].push(ch);
    });

    Object.keys(groups).sort().forEach(dir => {
        const groupEl = document.createElement('div');
        groupEl.className = 'git-group';

        const header = document.createElement('div');
        header.className = 'git-group-header';
        const headerLabel = document.createElement('span');
        headerLabel.textContent = '📂 ';
        const dirEl = document.createElement('span');
        dirEl.className = 'git-group-dir';
        dirEl.textContent = dir;
        header.appendChild(headerLabel);
        header.appendChild(dirEl);
        groupEl.appendChild(header);

        const itemsBox = document.createElement('div');
        itemsBox.className = 'git-group-items';

        groups[dir].forEach(ch => {
            const itemEl = document.createElement('div');
            itemEl.className = 'git-file-item' + (ch.staged ? ' staged' : '');
            itemEl.title = ch.path;

            const tag = document.createElement('span');
            tag.className = `status-tag ${ch.status}`;
            tag.textContent = ch.status;
            itemEl.appendChild(tag);

            const nameEl = document.createElement('span');
            nameEl.className = 'git-file-name';
            const lastSlash = ch.path.lastIndexOf('/');
            nameEl.textContent = lastSlash !== -1 ? ch.path.substring(lastSlash + 1) : ch.path;
            nameEl.title = `View diff: ${ch.path}`;
            nameEl.addEventListener('click', () => showDiff(workDir, ch.path));
            itemEl.appendChild(nameEl);

            const diffBtn = document.createElement('button');
            diffBtn.className = 'git-file-btn';
            diffBtn.textContent = '⤢';
            diffBtn.title = 'View diff';
            diffBtn.addEventListener('click', () => showDiff(workDir, ch.path));
            itemEl.appendChild(diffBtn);

            const stageBtn = document.createElement('button');
            stageBtn.className = 'git-file-btn' + (ch.staged ? ' stage-on' : '');
            stageBtn.textContent = ch.staged ? '−' : '＋';
            stageBtn.title = ch.staged ? 'Unstage' : 'Stage';
            stageBtn.addEventListener('click', () => gitStagePath(workDir, ch.path, ch.staged));
            itemEl.appendChild(stageBtn);

            itemsBox.appendChild(itemEl);
        });

        groupEl.appendChild(itemsBox);
        dom.gitChangesContainerEl.appendChild(groupEl);
    });
}

async function renderGitBranches(workDir, currentBranch) {
    if (!dom.gitBranchSelectEl) return;
    // Don't clobber the dropdown while the user is interacting with it.
    if (document.activeElement === dom.gitBranchSelectEl) return;

    let branches = [];
    try {
        branches = await callGitBranches(workDir);
    } catch (e) {
        return;
    }
    if (!Array.isArray(branches)) return;

    const selected = currentBranch || dom.gitBranchSelectEl.value;
    dom.gitBranchSelectEl.innerHTML = '';
    branches.forEach(b => {
        const opt = document.createElement('option');
        opt.value = b.name;
        opt.textContent = b.name + (b.current ? ' *' : '');
        if (b.name === selected) opt.selected = true;
        dom.gitBranchSelectEl.appendChild(opt);
    });
    if (branches.length === 0) {
        const opt = document.createElement('option');
        opt.value = '';
        opt.textContent = '(no branches)';
        dom.gitBranchSelectEl.appendChild(opt);
    }
}

async function renderGitLog(workDir) {
    if (!dom.gitLogContainerEl) return;
    let commits = [];
    try {
        commits = await callGitLog(workDir);
    } catch (e) {
        return;
    }
    if (!Array.isArray(commits)) return;

    dom.gitLogContainerEl.innerHTML = '';
    if (commits.length === 0) {
        dom.gitLogContainerEl.innerHTML = '<div class="git-empty">No commits yet.</div>';
        return;
    }

    commits.forEach(c => {
        const itemEl = document.createElement('div');
        itemEl.className = 'git-log-item' + (c.isHead ? ' is-head' : '');

        const top = document.createElement('div');
        top.className = 'git-log-top';

        const hashEl = document.createElement('span');
        hashEl.className = 'git-log-hash';
        hashEl.textContent = c.shortHash;
        hashEl.title = `View commit detail: ${c.hash}`;
        hashEl.addEventListener('click', () => showCommitDetail(workDir, c.hash));
        top.appendChild(hashEl);

        if (c.refs && c.refs.length > 0) {
            const refsEl = document.createElement('span');
            refsEl.className = 'git-log-refs';
            refsEl.textContent = c.refs.join(', ');
            top.appendChild(refsEl);
        }
        itemEl.appendChild(top);

        const msgEl = document.createElement('div');
        msgEl.className = 'git-log-msg';
        msgEl.textContent = c.subject;
        itemEl.appendChild(msgEl);

        const metaEl = document.createElement('div');
        metaEl.className = 'git-log-meta';
        metaEl.textContent = `${c.author} · ${c.date}`;
        itemEl.appendChild(metaEl);

        dom.gitLogContainerEl.appendChild(itemEl);
    });
}

function renderGitDiff(container, text) {
    container.textContent = '';
    if (!text) return;
    const lines = text.split('\n');
    const frag = document.createDocumentFragment();
    for (const line of lines) {
        const span = document.createElement('span');
        span.textContent = line;
        span.className = 'git-diff-line';
        if (/^@@ /.test(line)) {
            span.classList.add('git-diff-hunk');
        } else if (/^(diff |index |similarity |new file mode |deleted file mode |old mode |new mode |rename |copy |Binary files |\\)/.test(line)) {
            span.classList.add('git-diff-meta');
        } else if (/^--- /.test(line) || /^\+\+\+ /.test(line)) {
            span.classList.add('git-diff-meta');
        } else if (/^\+/.test(line)) {
            span.classList.add('git-diff-add');
        } else if (/^-/.test(line)) {
            span.classList.add('git-diff-del');
        } else if (/^=== /.test(line)) {
            span.classList.add('git-diff-sep');
        }
        frag.appendChild(span);
    }
    container.appendChild(frag);
}

let diffMinimapRegions = [];

function clearDiffMinimap() {
    diffMinimapRegions = [];
    if (!dom.gitDiffMinimapEl) return;
    dom.gitDiffMinimapEl.querySelectorAll('.git-diff-minimap-seg').forEach(s => s.remove());
    if (dom.gitDiffMinimapViewportEl) {
        dom.gitDiffMinimapViewportEl.style.top = '0px';
        dom.gitDiffMinimapViewportEl.style.height = '0px';
    }
}

function collectDiffRegions(container) {
    const regions = [];
    let current = null;
    container.querySelectorAll('.git-diff-line').forEach(line => {
        const isChange = line.classList.contains('git-diff-add') || line.classList.contains('git-diff-del');
        if (isChange) {
            if (!current) current = { els: [] };
            current.els.push(line);
        } else if (current) {
            regions.push(current);
            current = null;
        }
    });
    if (current) regions.push(current);
    return regions;
}

function updateDiffMinimapViewport() {
    if (!dom.gitDiffMinimapEl || !dom.gitDiffMinimapViewportEl || !dom.gitDiffContentEl) return;
    const container = dom.gitDiffContentEl;
    const scrollHeight = container.scrollHeight;
    const clientHeight = container.clientHeight;
    const minimapHeight = dom.gitDiffMinimapEl.clientHeight;
    if (!scrollHeight || !minimapHeight || clientHeight >= scrollHeight) {
        dom.gitDiffMinimapViewportEl.style.top = '0px';
        dom.gitDiffMinimapViewportEl.style.height = '0px';
        return;
    }
    const vh = Math.max(10, clientHeight / scrollHeight * minimapHeight);
    const vtop = container.scrollTop / scrollHeight * minimapHeight;
    dom.gitDiffMinimapViewportEl.style.top = `${vtop}px`;
    dom.gitDiffMinimapViewportEl.style.height = `${vh}px`;
}

function buildDiffMinimap(container) {
    clearDiffMinimap();
    if (!dom.gitDiffMinimapEl || !dom.gitDiffMinimapViewportEl || !container) return;
    const scrollHeight = container.scrollHeight;
    const clientHeight = container.clientHeight;
    const minimapHeight = dom.gitDiffMinimapEl.clientHeight;
    if (!scrollHeight || !minimapHeight || clientHeight >= scrollHeight) return;

    diffMinimapRegions = collectDiffRegions(container);
    const scale = minimapHeight / scrollHeight;
    for (const region of diffMinimapRegions) {
        const first = region.els[0];
        const last = region.els[region.els.length - 1];
        const top = first.offsetTop * scale;
        const bottom = (last.offsetTop + last.offsetHeight) * scale;
        const seg = document.createElement('div');
        seg.className = 'git-diff-minimap-seg';
        const hasAdd = region.els.some(el => el.classList.contains('git-diff-add'));
        const hasDel = region.els.some(el => el.classList.contains('git-diff-del'));
        seg.classList.add(hasAdd && hasDel ? 'is-mixed' : (hasAdd ? 'is-add' : 'is-del'));
        seg.style.top = `${top}px`;
        seg.style.height = `${Math.max(2, bottom - top)}px`;
        seg.title = `${region.els.length} changed line${region.els.length > 1 ? 's' : ''}`;
        seg.addEventListener('click', (e) => {
            e.stopPropagation();
            container.scrollTop = Math.max(0, first.offsetTop - 8);
            updateDiffMinimapViewport();
        });
        seg.addEventListener('mouseenter', () => {
            seg.classList.add('is-hover');
            region.els.forEach(el => el.classList.add('git-diff-hover'));
        });
        seg.addEventListener('mouseleave', () => {
            seg.classList.remove('is-hover');
            region.els.forEach(el => el.classList.remove('git-diff-hover'));
        });
        dom.gitDiffMinimapEl.insertBefore(seg, dom.gitDiffMinimapViewportEl);
    }
    updateDiffMinimapViewport();
}

function scrollDiffToFirstChange(container) {
    const first = container.querySelector('.git-diff-line.git-diff-add, .git-diff-line.git-diff-del');
    if (first) container.scrollTop = Math.max(0, first.offsetTop - 8);
}

async function showDiff(workDir, path) {
    if (!dom.gitDiffViewEl || !dom.gitDiffContentEl || !dom.gitDiffPathEl) return;
    const wasHidden = dom.gitDiffViewEl.classList.contains('hidden');
    selectedDiffPath = path;
    const res = await callGitDiff(workDir, path);
    if (res.error) {
        dom.gitDiffPathEl.textContent = path;
        dom.gitDiffContentEl.textContent = `Error: ${res.error}`;
        clearDiffMinimap();
    } else if (res.binary) {
        dom.gitDiffPathEl.textContent = path;
        dom.gitDiffContentEl.textContent = '(binary file — diff not available)';
        clearDiffMinimap();
    } else if (!res.staged && !res.unstaged) {
        dom.gitDiffPathEl.textContent = path;
        dom.gitDiffContentEl.textContent = '(no textual diff — new/untracked file)';
        clearDiffMinimap();
    } else {
        dom.gitDiffPathEl.textContent = path;
        const parts = [];
        if (res.staged) {
            parts.push('=== Staged ===\n' + res.staged);
        }
        if (res.unstaged) {
            parts.push('=== Unstaged ===\n' + res.unstaged);
        }
        renderGitDiff(dom.gitDiffContentEl, parts.join('\n'));
        dom.gitDiffViewEl.classList.remove('hidden');
        buildDiffMinimap(dom.gitDiffContentEl);
        if (wasHidden) scrollDiffToFirstChange(dom.gitDiffContentEl);
    }
    dom.gitDiffViewEl.classList.remove('hidden');
    updateDiffMinimapViewport();
}

async function showCommitDetail(workDir, hash) {
    if (!dom.gitCommitViewEl || !dom.gitCommitContentEl || !dom.gitCommitPathEl) return;
    selectedCommitHash = hash;
    selectedDiffPath = null;
    const res = await callGitShow(workDir, hash);
    if (res.error) {
        dom.gitCommitPathEl.textContent = hash;
        dom.gitCommitContentEl.textContent = `Error: ${res.error}`;
    } else {
        dom.gitCommitPathEl.textContent = `commit ${res.shortHash || hash}`;
        const header =
            `commit ${res.hash}\n` +
            `Author: ${res.author} <${res.email}>\n` +
            `Date:   ${res.date}\n` +
            `\n    ${res.message.replace(/\n/g, '\n    ')}\n`;
        renderGitDiff(dom.gitCommitContentEl, header + (res.diff || '(no diff)'));
    }
    dom.gitCommitViewEl.classList.remove('hidden');
    dom.gitCommitContentEl.scrollTop = 0;
}

export function closeDiffView() {
    selectedDiffPath = null;
    selectedCommitHash = null;
    if (dom.gitDiffViewEl) dom.gitDiffViewEl.classList.add('hidden');
    if (dom.gitDiffContentEl) dom.gitDiffContentEl.textContent = '';
    if (dom.gitCommitViewEl) dom.gitCommitViewEl.classList.add('hidden');
    if (dom.gitCommitContentEl) dom.gitCommitContentEl.textContent = '';
    clearDiffMinimap();
}

function showOpOutput(res) {
    if (!dom.gitOpOutputEl) return;
    if (!res) return;
    dom.gitOpOutputEl.classList.remove('hidden');
    if (res.success) {
        dom.gitOpOutputEl.className = 'git-op-output success';
        dom.gitOpOutputEl.textContent = res.output || 'Done.';
    } else {
        dom.gitOpOutputEl.className = 'git-op-output error';
        dom.gitOpOutputEl.textContent = res.error || res.output || 'Operation failed.';
    }
    dom.gitOpOutputEl.scrollTop = dom.gitOpOutputEl.scrollHeight;
}

export function initGitEvents() {
    addClick('btn-toggle-git', () => {
        dom.gitPanelEl.classList.toggle('hidden');
        if (!dom.gitPanelEl.classList.contains('hidden')) {
            updateGitStatus();
        }
    });

    addClick('btn-close-git', () => {
        dom.gitPanelEl.classList.add('hidden');
    });

    addClick('btn-git-refresh', updateGitStatus);

    // Git panel actions
    addClick('btn-git-fetch', () => gitRemoteAction('fetch'));
    addClick('btn-git-pull', () => gitRemoteAction('pull'));
    addClick('btn-git-push', () => gitRemoteAction('push'));
    addClick('btn-git-commit', () => gitCommitAction());
    addClick('btn-git-stage-all', () => gitStageAllAction());
    addClick('btn-close-diff', closeDiffView);
    addClick('btn-close-commit', closeDiffView);

    document.addEventListener('keydown', (e) => {
        if (e.key !== 'Escape') return;
        if (dom.gitDiffViewEl && !dom.gitDiffViewEl.classList.contains('hidden')) closeDiffView();
        else if (dom.gitCommitViewEl && !dom.gitCommitViewEl.classList.contains('hidden')) closeDiffView();
    });

    [dom.gitDiffViewEl, dom.gitCommitViewEl].forEach(el => {
        if (!el) return;
        el.addEventListener('click', (e) => {
            if (e.target === el) closeDiffView();
        });
    });

    if (dom.gitDiffContentEl) {
        dom.gitDiffContentEl.addEventListener('scroll', updateDiffMinimapViewport);
    }
    if (dom.gitDiffMinimapEl) {
        dom.gitDiffMinimapEl.addEventListener('click', (e) => {
            if (!dom.gitDiffContentEl) return;
            const rect = dom.gitDiffMinimapEl.getBoundingClientRect();
            if (rect.height <= 0) return;
            const ratio = (e.clientY - rect.top) / rect.height;
            dom.gitDiffContentEl.scrollTop = Math.max(0, ratio * (dom.gitDiffContentEl.scrollHeight - dom.gitDiffContentEl.clientHeight));
            updateDiffMinimapViewport();
        });
    }
    window.addEventListener('resize', () => {
        if (dom.gitDiffViewEl && dom.gitDiffContentEl && !dom.gitDiffViewEl.classList.contains('hidden')) {
            buildDiffMinimap(dom.gitDiffContentEl);
        }
    });

    const gitCommitInputEl = document.getElementById('git-commit-message');
    if (gitCommitInputEl) {
        gitCommitInputEl.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                gitCommitAction();
            }
        });
    }

    const gitBranchSelectEl = document.getElementById('git-branch-select');
    if (gitBranchSelectEl) {
        gitBranchSelectEl.addEventListener('change', async (e) => {
            const branch = e.target.value;
            const workDir = getActiveWorkDir();
            if (!workDir || !branch) return;
            const res = await callGitCheckout(workDir, branch);
            showOpOutput(res);
            await updateGitStatus();
        });
    }

    initGitPanelResizer();

    const pollSelectEl = document.getElementById('git-poll-interval-select');
    if (pollSelectEl) {
        pollSelectEl.addEventListener('change', async (e) => {
            const newInterval = parseInt(e.target.value, 10) || 3;
            state.currentGitPollInterval = newInterval;
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
}
