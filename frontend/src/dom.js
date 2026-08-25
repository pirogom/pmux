// DOM Elements
export const dom = {
    sessionListEl: document.getElementById('session-list'),
    profileListEl: document.getElementById('profile-list'),
    activeSessionTitleEl: document.getElementById('active-session-title'),
    btnRenameSessionEl: document.getElementById('btn-rename-session'),
    btnOpenWorkFolderEl: document.getElementById('btn-open-work-folder'),
    btnRefreshSessionPanesEl: document.getElementById('btn-refresh-session-panes'),
    terminalWorkspaceEl: document.getElementById('terminal-container'),
    emptyStateEl: document.getElementById('empty-state'),

    gitPanelEl: document.getElementById('git-panel'),
    gitBranchBadgeEl: document.getElementById('git-branch-badge'),
    gitChangesContainerEl: document.getElementById('git-changes-container'),
    gitChangesCountEl: document.getElementById('git-changes-count'),
    gitStagedCountEl: document.getElementById('git-staged-count'),
    gitLogContainerEl: document.getElementById('git-log-container'),
    gitOpOutputEl: document.getElementById('git-op-output'),
    gitDiffViewEl: document.getElementById('git-diff-view'),
    gitDiffContentEl: document.getElementById('git-diff-content'),
    gitDiffPathEl: document.getElementById('git-diff-path'),
    gitDiffMinimapEl: document.getElementById('git-diff-minimap'),
    gitDiffMinimapViewportEl: document.getElementById('git-diff-minimap-viewport'),
    gitCommitViewEl: document.getElementById('git-commit-view'),
    gitCommitContentEl: document.getElementById('git-commit-content'),
    gitCommitPathEl: document.getElementById('git-commit-path'),
    gitBranchSelectEl: document.getElementById('git-branch-select'),

    todoModalEl: document.getElementById('todo-modal'),
    todoListEl: document.getElementById('todo-list'),
    btnAddTodoEl: document.getElementById('btn-add-todo'),

    profileModalEl: document.getElementById('profile-modal'),
    profNameInput: document.getElementById('prof-name'),
    profCmdInput: document.getElementById('prof-cmd'),
    profArgsInput: document.getElementById('prof-args'),
    profDirInput: document.getElementById('prof-dir'),
    presetButtonsEl: document.getElementById('preset-buttons'),

    confirmModalEl: document.getElementById('confirm-modal'),
    confirmTitleEl: document.getElementById('confirm-title'),
    confirmMessageEl: document.getElementById('confirm-message'),
    btnConfirmOk: document.getElementById('btn-confirm-ok'),
    btnConfirmCancel: document.getElementById('btn-confirm-cancel'),

    shutdownModalEl: document.getElementById('shutdown-modal'),
    shutdownConfirmInput: document.getElementById('shutdown-confirm-input'),
    btnConfirmShutdown: document.getElementById('btn-confirm-shutdown'),
    btnCancelShutdown: document.getElementById('btn-cancel-shutdown'),
    btnCloseShutdownModal: document.getElementById('btn-close-shutdown-modal'),
    btnShutdownServer: document.getElementById('btn-shutdown-server'),

    toastContainerEl: document.getElementById('toast-container'),

    sshModalEl: document.getElementById('ssh-modal'),
    sshClientPathInput: document.getElementById('ssh-client-path'),
    sshAddressListEl: document.getElementById('ssh-address-list'),
    sshAddressModalEl: document.getElementById('ssh-address-modal'),
    sshAddrNameInput: document.getElementById('ssh-addr-name'),
    sshAddrDescInput: document.getElementById('ssh-addr-desc'),
    sshAddrHostInput: document.getElementById('ssh-addr-host'),
    sshAddrUserInput: document.getElementById('ssh-addr-user'),
    sshPassModalEl: document.getElementById('ssh-pass-modal'),
    sshPassTitleEl: document.getElementById('ssh-pass-title'),
    sshPassMessageEl: document.getElementById('ssh-pass-message'),
    sshPassInput: document.getElementById('ssh-pass-input'),
    sshPassConfirmGroupEl: document.getElementById('ssh-pass-confirm-group'),
    sshPassConfirmInput: document.getElementById('ssh-pass-confirm-input')
};

export function addClick(id, handler) {
    const el = document.getElementById(id);
    if (el) {
        el.addEventListener('click', handler);
    }
}
