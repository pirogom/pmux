import { defineConfig } from 'vite';

// Keep the terminal and rich-text editor out of the initial bundle. Both are
// large dependencies that are not needed until the user opens a pane/note.
export default defineConfig({
    build: {
        rollupOptions: {
            output: {
                manualChunks: {
                    xterm: ['@xterm/xterm', '@xterm/addon-webgl'],
                    quill: ['quill']
                }
            }
        }
    }
});
