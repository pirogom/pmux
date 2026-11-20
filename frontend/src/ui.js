import { dom, addClick } from './dom.js';
import { apiPost } from './api.js';

// Custom UI Components: Toast Notification & Custom Confirm Modal

// escapeHtml converts a value to text safe for innerHTML interpolation.
export function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
}

export function showToast(message, type = 'info') {
    // Log to console as well
    if (type === 'error') {
        console.error(`[pmux error] ${message}`);
    } else if (type === 'warning') {
        console.warn(`[pmux warn] ${message}`);
    } else {
        console.log(`[pmux info] ${message}`);
    }

    if (!dom.toastContainerEl) return;
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    const icon = type === 'error' ? '❌' : type === 'success' ? '✅' : 'ℹ️';
    // Build nodes instead of innerHTML: messages embed session names and
    // server error text, which must never be parsed as markup.
    const iconEl = document.createElement('span');
    iconEl.textContent = icon;
    const textEl = document.createElement('span');
    textEl.textContent = String(message);
    toast.appendChild(iconEl);
    toast.appendChild(document.createTextNode(' '));
    toast.appendChild(textEl);
    dom.toastContainerEl.appendChild(toast);

    setTimeout(() => {
        toast.style.animation = 'fadeOut 0.3s forwards';
        setTimeout(() => toast.remove(), 300);
    }, 3000);
}

export function fallbackCopyTextToClipboard(text) {
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

export function showConfirm(title, message) {
    return new Promise((resolve) => {
        dom.confirmTitleEl.textContent = title;
        dom.confirmMessageEl.textContent = message;
        dom.confirmModalEl.classList.remove('hidden');

        const onOk = () => {
            cleanup();
            resolve(true);
        };
        const onCancel = () => {
            cleanup();
            resolve(false);
        };
        const cleanup = () => {
            dom.confirmModalEl.classList.add('hidden');
            dom.btnConfirmOk.removeEventListener('click', onOk);
            dom.btnConfirmCancel.removeEventListener('click', onCancel);
        };

        dom.btnConfirmOk.addEventListener('click', onOk);
        dom.btnConfirmCancel.addEventListener('click', onCancel);
    });
}

export function openShutdownModal() {
    if (!dom.shutdownModalEl) return;
    dom.shutdownModalEl.classList.remove('hidden');
    if (dom.shutdownConfirmInput) {
        dom.shutdownConfirmInput.value = '';
        setTimeout(() => dom.shutdownConfirmInput.focus(), 50);
    }
    if (dom.btnConfirmShutdown) {
        dom.btnConfirmShutdown.disabled = true;
    }
}

export function closeShutdownModal() {
    if (!dom.shutdownModalEl) return;
    dom.shutdownModalEl.classList.add('hidden');
    if (dom.shutdownConfirmInput) {
        dom.shutdownConfirmInput.value = '';
    }
}

export async function executeShutdown() {
    if (dom.shutdownConfirmInput && dom.shutdownConfirmInput.value.trim() !== 'shutdown') {
        return;
    }
    closeShutdownModal();
    showToast('Shutting down pmux server and clients...', 'info');
    try {
        if (window.go && window.go.main && window.go.main.App && window.go.main.App.KillServer) {
            await window.go.main.App.KillServer();
        } else {
            await apiPost('/api/server/kill');
        }
    } catch (err) {
        console.warn('KillServer error / already closing:', err);
    }
}

export function initUiEvents() {
    addClick('btn-shutdown-server', openShutdownModal);
    addClick('btn-cancel-shutdown', closeShutdownModal);
    addClick('btn-close-shutdown-modal', closeShutdownModal);

    if (dom.shutdownConfirmInput) {
        dom.shutdownConfirmInput.addEventListener('input', (e) => {
            if (dom.btnConfirmShutdown) {
                dom.btnConfirmShutdown.disabled = (e.target.value.trim() !== 'shutdown');
            }
        });
        dom.shutdownConfirmInput.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && dom.shutdownConfirmInput.value.trim() === 'shutdown') {
                e.preventDefault();
                executeShutdown();
            } else if (e.key === 'Escape') {
                e.preventDefault();
                closeShutdownModal();
            }
        });
    }

    if (dom.btnConfirmShutdown) {
        dom.btnConfirmShutdown.addEventListener('click', executeShutdown);
    }

    if (dom.shutdownModalEl) {
        dom.shutdownModalEl.addEventListener('click', (e) => {
            if (e.target === dom.shutdownModalEl) {
                closeShutdownModal();
            }
        });
    }
}
