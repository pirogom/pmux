import { state } from './state.js';
import { refreshSessions, attachToSession, setActivePane, showEmptyState, layoutSignature } from './workspace.js';
import { refreshConfigAndProfiles } from './profiles.js';
import { applyGitPollInterval } from './git.js';
import { serverWsUrl } from './api.js';

let eventsWS = null;
let reconnectDelay = 1000;
let shuttingDown = false;

export function connectGlobalEventsWS() {
    if (!state.serverToken) {
        console.warn('[events ws] No daemon token available; not connecting.');
        return;
    }
    if (eventsWS) {
        const old = eventsWS;
        eventsWS = null;
        try { old.close(); } catch (e) {}
    }

    const ws = new WebSocket(serverWsUrl('/ws/events'));
    eventsWS = ws;

    ws.onopen = () => {
        console.log('[events ws] Connected to global events stream');
        reconnectDelay = 1000;
        refreshSessions();
        refreshConfigAndProfiles();
    };

    ws.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data);
            if (data.type === 'server_shutdown') {
                console.log('[events ws] Server shutting down. Quitting client...');
                // Do not reconnect to a server that is intentionally gone.
                shuttingDown = true;
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

                // Remember the active session's layout before refreshing so
                // unrelated session events (another session split/closed) do
                // not tear down and re-render the workspace.
                const prevActiveId = state.activeSession ? state.activeSession.id : null;
                const prevSig = state.activeSession ? layoutSignature(state.activeSession.layout) : '';

                refreshSessions().then(() => {
                    if (!state.activeSession) {
                        if (state.sessions.length > 0) attachToSession(state.sessions[0].id);
                        else showEmptyState();
                        return;
                    }

                    const activeId = state.activeSession.id;
                    const activeSig = layoutSignature(state.activeSession.layout);
                    if (activeId !== prevActiveId || activeSig !== prevSig) {
                        attachToSession(activeId);
                    }
                    // Otherwise refreshSessions already updated the title and
                    // session list; the terminal DOM is still valid.
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

    ws.onclose = () => {
        // A stale socket's close event must not schedule another reconnect.
        if (eventsWS !== ws) return;
        eventsWS = null;
        if (shuttingDown) return;
        setTimeout(connectGlobalEventsWS, reconnectDelay);
        reconnectDelay = Math.min(reconnectDelay * 2, 10000);
    };

    ws.onerror = (err) => {
        console.error('[events ws] error:', err);
    };
}
