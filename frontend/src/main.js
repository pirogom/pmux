import { state } from './state.js';
import { initUiEvents } from './ui.js';
import { connectGlobalEventsWS } from './events.js';
import { refreshConfigAndProfiles, initProfileEvents } from './profiles.js';
import { refreshSessions, initWorkspaceEvents } from './workspace.js';
import { initSshEvents } from './ssh.js';
import { initTodoEvents } from './todo.js';
import { initNoteEvents } from './note.js';
import { startGitPollTimer, initGitEvents } from './git.js';

// Initialization
let isAppInitialized = false;

window.addEventListener('DOMContentLoaded', async () => {
    initUiEvents();
    initWorkspaceEvents();
    initProfileEvents();
    initSshEvents();
    initTodoEvents();
    initNoteEvents();
    initGitEvents();

    const initConfig = async () => {
        if (isAppInitialized) return;
        try {
            if (window.go && window.go.main && window.go.main.App) {
                state.serverPort = await window.go.main.App.GetServerPort();
                state.detectedPresets = await window.go.main.App.GetDetectedProfiles();
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

    // Initial Git Status poll timer (default 3 seconds, or the saved interval if already restored)
    startGitPollTimer(state.currentGitPollInterval);
});
