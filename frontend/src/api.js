import { state } from './state.js';

export function isWails() {
    return !!(window.go && window.go.main && window.go.main.App);
}

// withToken adds the daemon bearer token to a fetch init object.
function withToken(init = {}) {
    const headers = new Headers(init.headers || {});
    if (state.serverToken) {
        headers.set('Authorization', `Bearer ${state.serverToken}`);
    }
    return { ...init, headers };
}

async function parseJSON(res) {
    if (!res.ok) {
        let detail = '';
        try {
            detail = (await res.text()).slice(0, 200);
        } catch (e) {
            // ignore
        }
        throw new Error(`HTTP ${res.status}${detail ? ': ' + detail : ''}`);
    }
    if (res.status === 204) return null;
    const text = await res.text();
    if (!text) return null;
    try {
        return JSON.parse(text);
    } catch (e) {
        throw new Error(`Invalid JSON response: ${text.slice(0, 200)}`);
    }
}

// API Calls (Server HTTP API Fallback)
export async function apiGet(path) {
    const res = await fetch(`http://127.0.0.1:${state.serverPort}${path}`, withToken());
    return await parseJSON(res);
}

export async function apiPost(path, body) {
    const res = await fetch(`http://127.0.0.1:${state.serverPort}${path}`, withToken({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body === undefined ? null : body)
    }));
    return await parseJSON(res);
}

// serverWsUrl builds an authenticated WebSocket URL. Browsers cannot set
// headers on the handshake, so the token is passed as a query parameter.
export function serverWsUrl(path) {
    const sep = path.includes('?') ? '&' : '?';
    const token = state.serverToken ? `${sep}token=${encodeURIComponent(state.serverToken)}` : '';
    return `ws://127.0.0.1:${state.serverPort}${path}${token}`;
}
