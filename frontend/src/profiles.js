import { state } from './state.js';
import { dom, addClick } from './dom.js';
import { apiGet, apiPost } from './api.js';
import { showToast, showConfirm } from './ui.js';
import { applyGitPollInterval } from './git.js';
import { createSessionFromProfile } from './workspace.js';

let editingProfileId = null;

const DRAG_PROFILE = 'application/x-pmux-profile';
const DRAG_FOLDER = 'application/x-pmux-folder';

const collapsedFolders = new Set(JSON.parse(localStorage.getItem('pmux_folder_collapsed') || '[]'));

function saveCollapsedState() {
    localStorage.setItem('pmux_folder_collapsed', JSON.stringify([...collapsedFolders]));
}

function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

export async function refreshConfigAndProfiles() {
    try {
        let loaded = null;
        let folders = [];
        try {
            loaded = await apiGet('/api/profiles');
            try {
                const cfg = await apiGet('/api/config');
                if (cfg && cfg.gitPollInterval) {
                    applyGitPollInterval(cfg.gitPollInterval);
                }
                if (cfg && Array.isArray(cfg.profileFolders)) {
                    folders = cfg.profileFolders;
                }
            } catch (e2) {
                console.warn('[pmux] Failed to load config from REST API:', e2);
            }
            // Stale daemon fallback: if the daemon does not know about
            // profileFolders yet, read folders from the local process config.
            if (folders.length === 0 && window.go && window.go.main && window.go.main.App && window.go.main.App.GetConfig) {
                try {
                    const cfg = await window.go.main.App.GetConfig();
                    if (cfg) {
                        folders = cfg.profileFolders || cfg.ProfileFolders || [];
                    }
                } catch (e3) {
                    console.warn('[pmux] Failed to load folders via Wails:', e3);
                }
            }
        } catch (e) {
            if (window.go && window.go.main && window.go.main.App) {
                if (window.go.main.App.GetConfig) {
                    const cfg = await window.go.main.App.GetConfig();
                    if (cfg) {
                        if (cfg.gitPollInterval) {
                            applyGitPollInterval(cfg.gitPollInterval);
                        }
                        loaded = cfg.profiles || cfg.Profiles || [];
                        folders = cfg.profileFolders || cfg.ProfileFolders || [];
                    }
                }
                if ((!loaded || loaded.length === 0) && window.go.main.App.GetProfiles) {
                    loaded = await window.go.main.App.GetProfiles();
                }
            }
        }
        state.profiles = loaded || [];
        state.profileFolders = folders || [];
        renderProfileList();
    } catch (e) {
        console.error('Failed to load profiles:', e);
    }
}

function renderProfileList() {
    dom.profileListEl.innerHTML = '';
    const profiles = state.profiles || [];
    const folders = state.profileFolders || [];

    if (profiles.length === 0 && folders.length === 0) {
        const emptyLi = document.createElement('li');
        emptyLi.textContent = 'No profiles';
        emptyLi.style.color = 'var(--text-muted)';
        emptyLi.style.cursor = 'default';
        dom.profileListEl.appendChild(emptyLi);
        return;
    }

    folders.forEach(folder => {
        renderFolderItem(folder, profiles);
    });

    profiles.forEach(prof => {
        if (!prof.folder) {
            renderProfileItem(prof);
        }
    });
}

function renderFolderItem(folder, profiles) {
    const li = document.createElement('li');
    li.className = 'profile-folder';
    li.dataset.folderId = folder.id;
    li.title = folder.name;

    const collapsed = collapsedFolders.has(folder.id);
    const memberCount = profiles.filter(p => p.folder === folder.id).length;

    li.innerHTML = `<span class="folder-chevron">${collapsed ? '▶' : '▼'}</span>
        <span class="folder-name">📁 ${escapeHtml(folder.name)}</span>
        <span class="folder-count">${memberCount}</span>
        <div class="item-actions">
            <button class="icon-btn-small folder-del-btn" title="Delete Folder">✖</button>
        </div>`;

    li.addEventListener('click', (e) => {
        if (e.target.closest('.item-actions')) return;
        if (e.target.closest('.folder-name')) return;
        if (collapsedFolders.has(folder.id)) {
            collapsedFolders.delete(folder.id);
        } else {
            collapsedFolders.add(folder.id);
        }
        saveCollapsedState();
        renderProfileList();
    });

    li.querySelector('.folder-name').addEventListener('dblclick', (e) => {
        e.stopPropagation();
        startRenameFolder(folder);
    });
    li.querySelector('.folder-del-btn').addEventListener('click', async (e) => {
        e.stopPropagation();
        await deleteFolder(folder);
    });

    setupFolderDrag(li, folder);
    dom.profileListEl.appendChild(li);

    if (collapsed) return;
    profiles.forEach(prof => {
        if (prof.folder === folder.id) {
            renderProfileItem(prof, true);
        }
    });
}

function renderProfileItem(prof, inFolder = false) {
    const li = document.createElement('li');
    li.className = inFolder ? 'profile-item in-folder' : 'profile-item';
    li.dataset.profileId = prof.id;
    li.dataset.folderId = prof.folder || '';
    li.title = prof.name;
    li.innerHTML = `<span>🐚 ${escapeHtml(prof.name)}</span>
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
    setupProfileDrag(li, prof);
    dom.profileListEl.appendChild(li);
}

function setupProfileDrag(li, prof) {
    li.draggable = true;
    li.addEventListener('dragstart', (e) => {
        e.dataTransfer.setData(DRAG_PROFILE, prof.id);
        e.dataTransfer.effectAllowed = 'move';
        li.classList.add('dragging');
    });
    li.addEventListener('dragend', clearDragState);
    setupDropTarget(li);
}

function setupFolderDrag(li, folder) {
    li.draggable = true;
    li.addEventListener('dragstart', (e) => {
        e.dataTransfer.setData(DRAG_FOLDER, folder.id);
        e.dataTransfer.effectAllowed = 'move';
        li.classList.add('dragging');
    });
    li.addEventListener('dragend', clearDragState);
    setupDropTarget(li);
}

function getDraggedKind(e) {
    if (!e.dataTransfer || !e.dataTransfer.types) return null;
    const types = Array.from(e.dataTransfer.types);
    if (types.includes(DRAG_PROFILE)) return 'profile';
    if (types.includes(DRAG_FOLDER)) return 'folder';
    return null;
}

function setupDropTarget(el) {
    el.addEventListener('dragover', (e) => {
        const kind = getDraggedKind(e);
        if (!kind) return;
        e.preventDefault();
        e.stopPropagation();
        e.dataTransfer.dropEffect = 'move';
        el.classList.remove('drag-over-before', 'drag-over-after', 'drag-over-folder');
        if (kind === 'profile' && el.classList.contains('profile-folder')) {
            el.classList.add('drag-over-folder');
            return;
        }
        const rect = el.getBoundingClientRect();
        const before = e.clientY < rect.top + rect.height / 2;
        el.classList.toggle('drag-over-before', before);
        el.classList.toggle('drag-over-after', !before);
    });
    el.addEventListener('dragleave', () => {
        el.classList.remove('drag-over-before', 'drag-over-after', 'drag-over-folder');
    });
    el.addEventListener('drop', (e) => {
        const kind = getDraggedKind(e);
        if (!kind) return;
        e.preventDefault();
        e.stopPropagation();
        const before = el.classList.contains('drag-over-before');
        if (kind === 'profile') {
            const profileId = e.dataTransfer.getData(DRAG_PROFILE);
            const targetFolder = el.dataset.folderId || '';
            let index;
            if (el.dataset.profileId !== undefined) {
                index = indexInFolderBefore(el.dataset.profileId, targetFolder, profileId);
                if (!before) index += 1;
            } else if (el.classList.contains('profile-folder')) {
                index = countInFolder(targetFolder, profileId);
                collapsedFolders.delete(targetFolder);
                saveCollapsedState();
            } else {
                index = countInFolder('', profileId);
            }
            moveProfile(profileId, targetFolder, index);
        } else {
            const folderId = e.dataTransfer.getData(DRAG_FOLDER);
            if (el.classList.contains('profile-folder')) {
                reorderFolder(folderId, el.dataset.folderId, before);
            }
        }
    });
}

function setupListDropTarget() {
    const ul = dom.profileListEl;
    ul.addEventListener('dragover', (e) => {
        const kind = getDraggedKind(e);
        if (!kind) return;
        e.preventDefault();
        e.stopPropagation();
        e.dataTransfer.dropEffect = 'move';
        ul.classList.add('drag-over-empty');
    });
    ul.addEventListener('dragleave', () => {
        ul.classList.remove('drag-over-empty');
    });
    ul.addEventListener('drop', (e) => {
        const kind = getDraggedKind(e);
        if (!kind) return;
        e.preventDefault();
        e.stopPropagation();
        const profileId = e.dataTransfer.getData(DRAG_PROFILE);
        if (profileId) {
            moveProfile(profileId, '', countInFolder('', profileId));
        } else {
            const folderId = e.dataTransfer.getData(DRAG_FOLDER);
            if (folderId) {
                reorderFolder(folderId, null, false);
            }
        }
    });
}

function clearDragState() {
    dom.profileListEl.classList.remove('drag-over-empty');
    dom.profileListEl.querySelectorAll('.dragging, .drag-over-before, .drag-over-after, .drag-over-folder').forEach(el => {
        el.classList.remove('dragging', 'drag-over-before', 'drag-over-after', 'drag-over-folder');
    });
}

function indexInFolderBefore(targetProfileId, folderId, draggedId) {
    let idx = 0;
    for (const p of state.profiles) {
        if (p.id === draggedId) continue;
        if ((p.folder || '') !== folderId) continue;
        if (p.id === targetProfileId) break;
        idx++;
    }
    return idx;
}

function countInFolder(folderId, draggedId) {
    let n = 0;
    for (const p of state.profiles) {
        if (p.id === draggedId) continue;
        if ((p.folder || '') === folderId) n++;
    }
    return n;
}

async function moveProfile(profileId, folderId, index) {
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.MoveProfile) {
            await window.go.main.App.MoveProfile(profileId, folderId, index);
        } else {
            await apiPost('/api/profiles/move', { profileId, folderId, index });
        }
        await refreshConfigAndProfiles();
    } catch (e) {
        showToast(`Failed to move profile: ${e.message || e}`, 'error');
        await refreshConfigAndProfiles();
    }
}

async function reorderFolder(draggedId, targetId, before) {
    if (targetId && draggedId === targetId) return;
    const ids = state.profileFolders.map(f => f.id);
    const from = ids.indexOf(draggedId);
    if (from < 0) return;
    ids.splice(from, 1);
    let to = ids.length;
    if (targetId) {
        const t = ids.indexOf(targetId);
        if (t < 0) return;
        to = before ? t : t + 1;
    }
    ids.splice(to, 0, draggedId);
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.ReorderProfileFolders) {
            await window.go.main.App.ReorderProfileFolders(ids);
        } else {
            await apiPost('/api/profile-folders/reorder', { ids });
        }
        await refreshConfigAndProfiles();
    } catch (e) {
        showToast(`Failed to reorder folders: ${e.message || e}`, 'error');
        await refreshConfigAndProfiles();
    }
}

function startRenameFolder(folder) {
    const row = dom.profileListEl.querySelector(`.profile-folder[data-folder-id="${CSS.escape(folder.id)}"]`);
    if (!row) return;
    const nameEl = row.querySelector('.folder-name');
    if (!nameEl) return;

    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'folder-name-input';
    input.value = folder.name;
    input.maxLength = 256;
    input.spellcheck = false;
    nameEl.replaceWith(input);
    input.focus();
    input.select();

    let done = false;
    const finish = async () => {
        if (done) return;
        done = true;
        const val = input.value.trim();
        if (val && val !== folder.name) {
            try {
                if (window.go && window.go.main && window.go.main.App && window.go.main.App.RenameProfileFolder) {
                    await window.go.main.App.RenameProfileFolder(folder.id, val);
                } else {
                    await apiPost('/api/profile-folders/rename', { id: folder.id, name: val });
                }
                await refreshConfigAndProfiles();
                showToast('Folder renamed', 'success');
            } catch (e) {
                showToast(`Failed to rename folder: ${e.message || e}`, 'error');
                renderProfileList();
            }
        } else {
            renderProfileList();
        }
    };
    const cancel = () => {
        if (done) return;
        done = true;
        renderProfileList();
    };

    input.addEventListener('keydown', (e) => {
        e.stopPropagation();
        if (e.key === 'Enter') {
            e.preventDefault();
            finish();
        } else if (e.key === 'Escape') {
            e.preventDefault();
            cancel();
        }
    });
    input.addEventListener('blur', finish);
    input.addEventListener('click', (e) => e.stopPropagation());
}

async function addFolder() {
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.CreateProfileFolder) {
            await window.go.main.App.CreateProfileFolder('New Folder');
        } else {
            await apiPost('/api/profile-folders/create', { name: 'New Folder' });
        }
        await refreshConfigAndProfiles();
        const folders = state.profileFolders;
        if (folders.length > 0) {
            startRenameFolder(folders[folders.length - 1]);
        }
        showToast('Folder created', 'success');
    } catch (e) {
        showToast(`Failed to create folder: ${e.message || e}`, 'error');
    }
}

async function deleteFolder(folder) {
    const confirmed = await showConfirm(
        'Delete Folder',
        `Delete folder "${folder.name}"?\nProfiles inside it will be moved out to the root list.`
    );
    if (!confirmed) return;
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.DeleteProfileFolder) {
            await window.go.main.App.DeleteProfileFolder(folder.id);
        } else {
            await apiPost('/api/profile-folders/delete', { id: folder.id });
        }
        await refreshConfigAndProfiles();
        showToast('Folder deleted', 'success');
    } catch (e) {
        showToast(`Failed to delete folder: ${e.message || e}`, 'error');
    }
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

export async function openProfileModal(profToEdit = null) {
    dom.profileModalEl.classList.remove('hidden');
    dom.presetButtonsEl.innerHTML = '';

    const modalTitle = document.getElementById('modal-title');
    if (profToEdit) {
        editingProfileId = profToEdit.id;
        if (modalTitle) modalTitle.textContent = 'Edit Profile';
        dom.profNameInput.value = profToEdit.name || '';
        dom.profCmdInput.value = profToEdit.command || '';
        dom.profArgsInput.value = (profToEdit.args || []).join(',');
        dom.profDirInput.value = profToEdit.workDir || '';
    } else {
        editingProfileId = null;
        if (modalTitle) modalTitle.textContent = 'Create New Profile';
        dom.profNameInput.value = '';
        dom.profCmdInput.value = '';
        dom.profArgsInput.value = '';
        dom.profDirInput.value = '';
    }

    if (!state.detectedPresets || state.detectedPresets.length === 0) {
        try {
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetDetectedProfiles) {
                state.detectedPresets = await window.go.main.App.GetDetectedProfiles();
            }
        } catch (e) {
            console.error('Failed to get detected presets:', e);
        }
    }

    if (state.detectedPresets && state.detectedPresets.length > 0) {
        state.detectedPresets.forEach(preset => {
            const btn = document.createElement('button');
            btn.className = 'tag-btn';
            btn.textContent = preset.name;
            btn.type = 'button';
            btn.addEventListener('click', () => {
                dom.profNameInput.value = preset.name;
                dom.profCmdInput.value = preset.command;
                dom.profArgsInput.value = (preset.args || []).join(',');
            });
            dom.presetButtonsEl.appendChild(btn);
        });
    } else {
        const span = document.createElement('span');
        span.style.color = 'var(--text-muted)';
        span.style.fontSize = '0.85rem';
        span.textContent = 'No presets detected automatically';
        dom.presetButtonsEl.appendChild(span);
    }
}

export function closeProfileModal() {
    editingProfileId = null;
    dom.profileModalEl.classList.add('hidden');
}

async function saveProfileFromModal() {
    let name = dom.profNameInput.value.trim();
    const command = dom.profCmdInput.value.trim();
    const argsStr = dom.profArgsInput.value.trim();
    const workDir = dom.profDirInput.value.trim();

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

export function initProfileEvents() {
    setupListDropTarget();

    addClick('btn-new-session-quick', () => {
        if (state.profiles.length > 0) {
            createSessionFromProfile(state.profiles[0]);
        } else {
            showToast('Please create a profile first', 'info');
            openProfileModal();
        }
    });

    addClick('btn-add-profile', () => openProfileModal());
    addClick('btn-add-folder', () => addFolder());

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
}
