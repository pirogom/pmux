import { state } from './state.js';
import { dom, addClick } from './dom.js';
import { apiGet, apiPost } from './api.js';
import { showToast, showConfirm } from './ui.js';
import { applyGitPollInterval } from './git.js';
import { createSessionFromProfile } from './workspace.js';

let editingProfileId = null;

export async function refreshConfigAndProfiles() {
    try {
        let loaded = null;
        // Query central daemon server REST API first to guarantee 0.01s real-time multi-client sync
        try {
            loaded = await apiGet('/api/profiles');
            // Restore the saved git poll interval (REST path also covers the desktop app,
            // so the Wails fallback below is only reached when the daemon is unreachable)
            try {
                const cfg = await apiGet('/api/config');
                if (cfg && cfg.gitPollInterval) {
                    applyGitPollInterval(cfg.gitPollInterval);
                }
            } catch (e2) {
                console.warn('[pmux] Failed to load config from REST API:', e2);
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
                    }
                }
                if ((!loaded || loaded.length === 0) && window.go.main.App.GetProfiles) {
                    loaded = await window.go.main.App.GetProfiles();
                }
            }
        }
        state.profiles = loaded || [];
        console.log('[pmux profiles loaded]', state.profiles);
        renderProfileList();
    } catch (e) {
        console.error('Failed to load profiles:', e);
    }
}

function renderProfileList() {
    dom.profileListEl.innerHTML = '';
    if (!state.profiles || state.profiles.length === 0) {
        const emptyLi = document.createElement('li');
        emptyLi.textContent = 'No profiles';
        emptyLi.style.color = 'var(--text-muted)';
        emptyLi.style.cursor = 'default';
        dom.profileListEl.appendChild(emptyLi);
        return;
    }

    state.profiles.forEach(prof => {
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
        dom.profileListEl.appendChild(li);
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
    addClick('btn-new-session-quick', () => {
        if (state.profiles.length > 0) {
            createSessionFromProfile(state.profiles[0]);
        } else {
            showToast('Please create a profile first', 'info');
            openProfileModal();
        }
    });

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
}
