import { state } from './state.js';

let reflowDebounceTimer = null;
let resizeRedrawTimer = null;

// scheduleResizeRedraw debounces a forced full repaint until a resize gesture
// settles. A plain ConPTY resize only emits a reflow/partial repaint, which
// can leave stale or duplicated lines in the client buffer for full-screen
// TUI apps (claude, agy, ...). The same forced redraw used by the Refresh
// Session Panes button resynchronizes the screen once resizing stops.
function scheduleResizeRedraw() {
    if (resizeRedrawTimer) {
        clearTimeout(resizeRedrawTimer);
    }
    resizeRedrawTimer = setTimeout(() => {
        resizeRedrawTimer = null;
        reflowAllPanes(true, true);
    }, 200);
}

// getCellSize returns the terminal's measured cell size in pixels, or null
// until the renderer has computed its dimensions. It relies only on public
// xterm.js APIs/DOM (the .xterm-screen element is sized to the buffer's exact
// pixel dimensions) instead of the private _core._renderService internals,
// which change between xterm releases.
export function getCellSize(term) {
    if (!term) return null;

    const measure = () => {
        if (!term.element || term.cols <= 0 || term.rows <= 0) return null;
        const screenEl = term.element.querySelector('.xterm-screen');
        if (!screenEl) return null;
        const w = screenEl.clientWidth;
        const h = screenEl.clientHeight;
        if (w > 0 && h > 0) {
            return { cellW: w / term.cols, cellH: h / term.rows };
        }
        return null;
    };

    let cell = measure();
    if (!cell) {
        // Force a render pass so the renderer sizes .xterm-screen.
        try {
            term.refresh(0, Math.max(0, term.rows - 1));
        } catch (e) {}
        cell = measure();
    }
    return cell;
}

// measureViewport computes this client's own viewport size (cols x rows) from
// its container's pixel dimensions. This is the size reported to the server
// via the resize message; the terminal buffer itself follows the pane's
// canonical size instead.
export function measureViewport(paneObj) {
    const { element, term } = paneObj;
    if (!element || !element.isConnected) return null;
    const rect = element.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return null;
    const cell = getCellSize(term);
    if (!cell) return null;
    const cols = Math.max(10, Math.floor(rect.width / cell.cellW));
    const rows = Math.max(3, Math.floor(rect.height / cell.cellH));
    return { cols, rows };
}

// applyViewportClip marks a pane whose canonical buffer is larger than this
// client's viewport. The buffer overflows the container and is clipped by CSS
// (top-left anchored, tmux-style viewport); the clipped classes hide the
// native scrollbar when it would sit outside the visible area.
export function applyViewportClip(termBox, term) {
    if (!termBox || !term || !term.element) return;
    const rect = termBox.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return;
    const cell = getCellSize(term);
    if (!cell) return;
    const clippedX = cell.cellW * term.cols > rect.width + 1;
    const clippedY = cell.cellH * term.rows > rect.height + 1;
    termBox.classList.toggle('clipped-x', clippedX);
    termBox.classList.toggle('clipped-y', clippedY);
    const badge = termBox.querySelector('.viewport-badge');
    if (badge) {
        const vpCols = Math.floor(rect.width / cell.cellW);
        const vpRows = Math.floor(rect.height / cell.cellH);
        badge.textContent = (clippedX || clippedY)
            ? `viewport ${vpCols}x${vpRows} / ${term.cols}x${term.rows}`
            : '';
    }
}

// sendViewportResize reports this client's viewport size to the server. The
// server only resizes the underlying ConPTY when the canonical (largest
// client) size changes, so ordinary viewport changes never redraw the
// terminal for everyone. If the measurement is not ready yet (layout or
// renderer still initializing) or the socket is not open, it retries a few
// times so a freshly attached client always propagates its real size.
export function sendViewportResize(paneObj, force = false, retries = 5) {
    if (!paneObj || !paneObj.element || !paneObj.element.isConnected || retries < 0) return;
    const size = measureViewport(paneObj);
    const ws = paneObj.ws;
    const canSend = size && ws && ws.readyState === WebSocket.OPEN;
    if (!canSend) {
        if (retries > 0) {
            setTimeout(() => sendViewportResize(paneObj, force, retries - 1), 100);
        }
        return;
    }
    if (!force && size.cols === paneObj.viewportCols && size.rows === paneObj.viewportRows) {
        return;
    }
    paneObj.viewportCols = size.cols;
    paneObj.viewportRows = size.rows;
    ws.send(JSON.stringify({ type: 'resize', cols: size.cols, rows: size.rows }));
}

export function reflowAllPanes(forceSendResize = false, forceRedraw = false) {
    if (reflowDebounceTimer) {
        clearTimeout(reflowDebounceTimer);
    }
    reflowDebounceTimer = setTimeout(() => {
        state.activePanesMap.forEach((paneObj) => {
            const { term, ws, element } = paneObj;
            try {
                if (element && element.isConnected && element.offsetWidth > 0 && element.offsetHeight > 0) {
                    applyViewportClip(element, term);
                    const size = measureViewport(paneObj);
                    if (ws && ws.readyState === WebSocket.OPEN && size) {
                        if (forceRedraw) {
                            // Explicit user-requested redraw: report the real
                            // viewport size; the server redraws at canonical.
                            paneObj.viewportCols = size.cols;
                            paneObj.viewportRows = size.rows;
                            ws.send(JSON.stringify({ type: 'redraw', cols: size.cols, rows: size.rows }));
                        } else {
                            const changed = size.cols !== paneObj.viewportCols || size.rows !== paneObj.viewportRows;
                            sendViewportResize(paneObj, forceSendResize);
                            if (changed) {
                                scheduleResizeRedraw();
                            }
                        }
                    }
                }
            } catch(e) {}
        });
    }, 30);
}
