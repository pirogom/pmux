import Quill from 'quill';
import 'quill/dist/quill.snow.css';
import { getActiveWorkDir } from './state.js';
import { dom, addClick } from './dom.js';
import { apiPost } from './api.js';
import { showToast, showConfirm } from './ui.js';

// --- Notes ---
let noteItems = [];
let noteWorkDir = '';
let activeNoteId = null;
let noteSaveTimer = null;
let quill = null;

const NOTE_FONT_SIZES = ['12px', '14px', '16px', '18px', '20px', '24px', '28px'];

async function callLoadNotes(workDir) {
    const res = await apiPost('/api/note/load', { workDir });
    return Array.isArray(res) ? res : [];
}

async function callSaveNotes(workDir, notes) {
    const res = await apiPost('/api/note/save', { workDir, notes });
    if (!res || res.success !== true) {
        throw new Error('Failed to save note list');
    }
}

function noteNewId() {
    return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

function createDefaultNote() {
    return { id: noteNewId(), title: 'New Note', content: '' };
}

function ensureAtLeastOneNote() {
    if (noteItems.length === 0) {
        noteItems.push(createDefaultNote());
        return true;
    }
    return false;
}

function activeNote() {
    return noteItems.find(n => n.id === activeNoteId) || null;
}

function initQuill() {
    if (quill || !dom.noteContentEl) return;

    const SizeAttributor = Quill.import('attributors/style/size');
    SizeAttributor.whitelist = NOTE_FONT_SIZES;
    Quill.register(SizeAttributor, true);

    quill = new Quill('#note-content', {
        theme: 'snow',
        modules: {
            toolbar: {
                container: '#note-toolbar'
            }
        },
        placeholder: 'Write your note here...'
    });

    quill.on('text-change', () => {
        syncActiveNoteFromDom();
        scheduleNoteSave();
    });
}

export async function openNoteModal() {
    const workDir = getActiveWorkDir();
    if (!workDir) {
        showToast('No work folder for the active pane. Notes are unavailable.', 'error');
        return;
    }
    noteWorkDir = workDir;
    try {
        noteItems = await callLoadNotes(workDir);
    } catch (e) {
        console.error('Failed to load notes:', e);
        noteItems = [];
        showToast('Failed to load notes.', 'error');
    }
    const created = ensureAtLeastOneNote();
    activeNoteId = noteItems[0].id;
    renderNoteList();
    renderActiveNote();
    if (dom.noteModalEl) dom.noteModalEl.classList.remove('hidden');
    if (created) {
        await flushNoteSave();
    }
    quill.focus();
}

export function closeNoteModal() {
    flushNoteSave();
    dom.noteTitleInput.blur();
    if (dom.noteModalEl) dom.noteModalEl.classList.add('hidden');
}

function scheduleNoteSave() {
    if (noteSaveTimer) {
        clearTimeout(noteSaveTimer);
    }
    noteSaveTimer = setTimeout(flushNoteSave, 500);
}

async function flushNoteSave() {
    if (noteSaveTimer) {
        clearTimeout(noteSaveTimer);
        noteSaveTimer = null;
    }
    if (!noteWorkDir) return;
    try {
        await callSaveNotes(noteWorkDir, noteItems);
    } catch (e) {
        console.error('Failed to save notes:', e);
        showToast('Failed to save notes.', 'error');
    }
}

function syncActiveNoteFromDom() {
    const note = activeNote();
    if (!note) return;
    note.title = dom.noteTitleInput.value.trim() || 'New Note';
    note.content = quill ? quill.getSemanticHTML() : '';
}

function renderNoteList() {
    if (!dom.noteListEl) return;
    dom.noteListEl.innerHTML = '';
    noteItems.forEach(item => {
        const row = document.createElement('div');
        row.className = 'note-list-item' + (item.id === activeNoteId ? ' active' : '');
        row.dataset.id = item.id;
        row.title = item.title || 'New Note';

        const titleSpan = document.createElement('span');
        titleSpan.className = 'note-list-title';
        titleSpan.textContent = item.title || 'New Note';
        row.appendChild(titleSpan);

        const delBtn = document.createElement('button');
        delBtn.className = 'note-list-del';
        delBtn.textContent = '✖';
        delBtn.title = 'Delete Note';
        delBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            await deleteNote(item.id);
        });
        row.appendChild(delBtn);

        row.addEventListener('click', () => selectNote(item.id));
        dom.noteListEl.appendChild(row);
    });
}

function renderActiveNote() {
    const note = activeNote();
    if (!note) {
        dom.noteTitleInput.value = '';
        if (quill) quill.setText('');
        return;
    }
    dom.noteTitleInput.value = note.title || '';
    if (quill) {
        quill.setContents(quill.clipboard.convert({ html: note.content || '' }));
    }
}

function selectNote(id) {
    syncActiveNoteFromDom();
    activeNoteId = id;
    renderNoteList();
    renderActiveNote();
    scheduleNoteSave();
    quill.focus();
}

async function noteAddItem() {
    syncActiveNoteFromDom();
    const newNote = createDefaultNote();
    noteItems.unshift(newNote);
    activeNoteId = newNote.id;
    renderNoteList();
    renderActiveNote();
    await flushNoteSave();
    dom.noteTitleInput.focus();
    dom.noteTitleInput.select();
}

async function deleteNote(id) {
    const target = noteItems.find(n => n.id === id);
    if (!target) return;
    const confirmed = await showConfirm('Delete Note', 'Are you sure you want to delete this note?');
    if (!confirmed) return;
    const index = noteItems.findIndex(n => n.id === id);
    noteItems.splice(index, 1);
    ensureAtLeastOneNote();
    activeNoteId = noteItems[0].id;
    renderNoteList();
    renderActiveNote();
    await flushNoteSave();
    showToast('Note deleted.', 'success');
}

export function initNoteEvents() {
    initQuill();

    addClick('btn-toggle-note', () => openNoteModal());
    addClick('btn-close-note', closeNoteModal);
    addClick('btn-note-add', () => noteAddItem());

    if (dom.noteTitleInput) {
        dom.noteTitleInput.addEventListener('input', () => {
            syncActiveNoteFromDom();
            renderNoteList();
            scheduleNoteSave();
        });
    }
}
