import { state } from './state.js';

export function isWails() {
    return !!(window.go && window.go.main && window.go.main.App);
}

// API Calls (Server HTTP API Fallback)
export async function apiGet(path) {
    const res = await fetch(`http://127.0.0.1:${state.serverPort}${path}`);
    return await res.json();
}

export async function apiPost(path, body) {
    const res = await fetch(`http://127.0.0.1:${state.serverPort}${path}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
    });
    return await res.json();
}
