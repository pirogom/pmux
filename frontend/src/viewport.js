import { state } from './state.js';

let reflowDebounceTimer = null;

// getCellSize returns the terminal's measured cell size in pixels, or null
// until the renderer has computed its dimensions.
export function getCellSize(term) {
    if (!term) return null;
    const readFromRenderService = () => {
        try {
            const core = term._core;
            if (core && core._renderService && core._renderService.dimensions && core._renderService.dimensions.css) {
                const css = core._renderService.dimensions.css;
                if (css.cell && css.cell.width > 0 && css.cell.height > 0) {
                    return { cellW: css.cell.width, cellH: css.cell.height };
                }
            }
        } catch (e) {}
        return null;
    };
    let cell = readFromRenderService();
    if (!cell) {
        // Force the renderer to measure the font metrics (like FitAddon does)
        try {
            const core = term._core;
            if (core && core._renderService && typeof core._renderService.measure === 'function') {
                core._renderService.measure();
            }
        } catch (e) {}
        cell = readFromRenderService();
    }
    if (!cell && term.element && term.cols > 0 && term.rows > 0) {
        // Fallback: .xterm-screen carries the buffer's exact pixel size
        // (unlike .xterm, which stretches to the container width), so the
        // derived cell size is font-accurate even before the first render.
        const screenEl = term.element.querySelector('.xterm-screen');
        if (screenEl && screenEl.clientWidth > 0 && screenEl.clientHeight > 0) {
            return { cellW: screenEl.clientWidth / term.cols, cellH: screenEl.clientHeight / term.rows };
        }
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
                            sendViewportResize(paneObj, forceSendResize);
                        }
                    }
                }
            } catch(e) {}
        });
    }, 30);
}
