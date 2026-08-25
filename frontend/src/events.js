import { state } from './state.js';
import { refreshSessions, attachToSession, setActivePane, showEmptyState } from './workspace.js';
import { refreshConfigAndProfiles } from './profiles.js';
import { applyGitPollInterval } from './git.js';

let eventsWS = null;

export function connectGlobalEventsWS() {
    if (eventsWS) {
        try { eventsWS.close(); } catch(e) {}
    }
    const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//127.0.0.1:${state.serverPort}/ws/events`;
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
                    if (state.activeSession && payload.sessionId === state.activeSession.id) {
                        state.activeSession.activePaneId = payload.paneId;
                        if (state.activePaneId !== payload.paneId) {
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
                    if (state.activeSession) {
                        const latestSess = state.sessions.find(s => s.id === state.activeSession.id);
                        if (latestSess) {
                            attachToSession(latestSess.id);
                        } else if (state.sessions.length > 0) {
                            attachToSession(state.sessions[0].id);
                        } else {
                            showEmptyState();
                        }
                    } else if (state.sessions.length > 0) {
                        attachToSession(state.sessions[0].id);
                    } else {
                        showEmptyState();
                    }
                });
            } else if (data.type === 'profiles_updated' || data.type === 'config_updated') {
                if (data.type === 'config_updated' && data.data && data.data.gitPollInterval) {
                    applyGitPollInterval(data.data.gitPollInterval);
                }
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
