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

// fetchWithTimeout aborts a request that takes too long. Without this a hung
// daemon would leave polling guards (e.g. gitStatusInFlight) stuck forever.
async function fetchWithTimeout(url, init, timeoutMs) {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    try {
        return await fetch(url, { ...init, signal: controller.signal });
    } catch (e) {
        if (e && e.name === 'AbortError') {
            throw new Error(`Request timed out after ${Math.round(timeoutMs / 1000)}s`);
        }
        throw e;
    } finally {
        clearTimeout(timer);
    }
}

const GET_TIMEOUT_MS = 30000;
const POST_TIMEOUT_MS = 120000;

// API Calls (Server HTTP API Fallback)
export async function apiGet(path) {
    const res = await fetchWithTimeout(`http://127.0.0.1:${state.serverPort}${path}`, withToken(), GET_TIMEOUT_MS);
    return await parseJSON(res);
}

export async function apiPost(path, body) {
    const res = await fetchWithTimeout(`http://127.0.0.1:${state.serverPort}${path}`, withToken({
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body === undefined ? null : body)
    }), POST_TIMEOUT_MS);
    return await parseJSON(res);
}

// serverWsUrl builds an authenticated WebSocket URL. Browsers cannot set
// headers on the handshake, so the token is passed as a query parameter.
export function serverWsUrl(path) {
    const sep = path.includes('?') ? '&' : '?';
    const token = state.serverToken ? `${sep}token=${encodeURIComponent(state.serverToken)}` : '';
    return `ws://127.0.0.1:${state.serverPort}${path}${token}`;
}
