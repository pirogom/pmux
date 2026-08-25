// Shared application state
export const state = {
    serverPort: 4799,
    activeSession: null,
    activePaneId: null,
    sessions: [],
    profiles: [],
    detectedPresets: [],
    activePanesMap: new Map(), // paneId -> { term, ws, element }
    currentGitPollInterval: 3
};

export function getActiveWorkDir() {
    if (!state.activePaneId) return '';
    const paneInfo = state.activePanesMap.get(state.activePaneId);
    return paneInfo ? paneInfo.workDir : '';
}
