import { getActiveWorkDir } from './state.js';
import { dom, addClick } from './dom.js';
import { apiPost } from './api.js';
import { showToast, showConfirm } from './ui.js';

// --- Todo List ---
let todoItems = [];
let todoWorkDir = '';
let todoDragIndex = null;

async function callLoadTodo(workDir) {
    const res = await apiPost('/api/todo/load', { workDir });
    return Array.isArray(res) ? res : [];
}

async function callSaveTodo(workDir, items) {
    const res = await apiPost('/api/todo/save', { workDir, items });
    if (!res || res.success !== true) {
        throw new Error('Failed to save todo list');
    }
}

function todoNewId() {
    return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
}

export async function openTodoModal() {
    const workDir = getActiveWorkDir();
    if (!workDir) {
        showToast('No work folder for the active pane. Todo list is unavailable.', 'error');
        return;
    }
    todoWorkDir = workDir;
    try {
        todoItems = await callLoadTodo(workDir);
    } catch (e) {
        console.error('Failed to load todo list:', e);
        todoItems = [];
        showToast('Failed to load todo list.', 'error');
    }
    renderTodoList();
    if (dom.todoModalEl) dom.todoModalEl.classList.remove('hidden');
}

export function closeTodoModal() {
    if (dom.todoModalEl) dom.todoModalEl.classList.add('hidden');
}

async function saveTodo() {
    if (!todoWorkDir) return;
    try {
        await callSaveTodo(todoWorkDir, todoItems);
    } catch (e) {
        console.error('Failed to save todo list:', e);
        showToast('Failed to save todo list.', 'error');
    }
}

async function todoDeleteAll() {
    if (todoItems.length === 0) {
        showToast('No todos to delete.', 'info');
        return;
    }
    const confirmed = await showConfirm('Delete All Todos', `Are you sure you want to delete all ${todoItems.length} todos?`);
    if (!confirmed) return;
    todoItems = [];
    renderTodoList();
    await saveTodo();
    showToast('All todos deleted.', 'success');
}

async function todoAddItem() {
    todoItems.push({
        id: todoNewId(),
        content: 'New Todo',
        done: false,
        strikethrough: false
    });
    renderTodoList();
    await saveTodo();
    startTodoEdit(todoItems.length - 1);
}

function renderTodoList() {
    if (!dom.todoListEl) return;
    dom.todoListEl.innerHTML = '';
    if (todoItems.length === 0) {
        const empty = document.createElement('div');
        empty.className = 'todo-empty';
        empty.textContent = 'No todos yet. Click Add to create one.';
        dom.todoListEl.appendChild(empty);
        return;
    }
    todoItems.forEach((item, index) => {
        const row = document.createElement('div');
        row.className = 'todo-item';
        row.draggable = true;
        row.dataset.index = String(index);

        row.addEventListener('dragstart', (e) => {
            todoDragIndex = index;
            row.classList.add('todo-dragging');
            e.dataTransfer.effectAllowed = 'move';
            try { e.dataTransfer.setData('text/plain', String(index)); } catch (err) {}
        });
        row.addEventListener('dragend', () => {
            todoDragIndex = null;
            row.classList.remove('todo-dragging');
            dom.todoListEl.querySelectorAll('.todo-item').forEach(el => el.classList.remove('todo-drag-over'));
        });
        row.addEventListener('dragover', (e) => {
            e.preventDefault();
            e.dataTransfer.dropEffect = 'move';
            row.classList.add('todo-drag-over');
        });
        row.addEventListener('dragleave', () => {
            row.classList.remove('todo-drag-over');
        });
        row.addEventListener('drop', async (e) => {
            e.preventDefault();
            row.classList.remove('todo-drag-over');
            const from = todoDragIndex;
            const to = index;
            if (from === null || from === undefined || from === to) return;
            const [moved] = todoItems.splice(from, 1);
            todoItems.splice(to, 0, moved);
            renderTodoList();
            await saveTodo();
        });

        const indexSpan = document.createElement('span');
        indexSpan.className = 'todo-index';
        indexSpan.textContent = String(index + 1);

        const checkbox = document.createElement('input');
        checkbox.type = 'checkbox';
        checkbox.className = 'todo-checkbox';
        checkbox.checked = item.done;
        checkbox.title = 'Mark as done';
        checkbox.addEventListener('change', async () => {
            item.done = checkbox.checked;
            await saveTodo();
        });

        const content = document.createElement('span');
        content.className = 'todo-content' + (item.strikethrough ? ' struck' : '');
        content.textContent = item.content;
        content.title = 'Double-click to edit';
        content.addEventListener('dblclick', (e) => {
            e.stopPropagation();
            startTodoEdit(index);
        });

        const strikeBtn = document.createElement('button');
        strikeBtn.className = 'todo-btn';
        strikeBtn.textContent = 'C';
        strikeBtn.title = 'Toggle strikethrough';
        strikeBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            item.strikethrough = !item.strikethrough;
            content.classList.toggle('struck', item.strikethrough);
            await saveTodo();
        });

        const delBtn = document.createElement('button');
        delBtn.className = 'todo-btn todo-btn-del';
        delBtn.textContent = 'X';
        delBtn.title = 'Delete';
        delBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            const confirmed = await showConfirm('Delete Todo', `Are you sure you want to delete "${item.content}"?`);
            if (!confirmed) return;
            todoItems.splice(index, 1);
            renderTodoList();
            await saveTodo();
        });

        row.append(indexSpan, checkbox, content, strikeBtn, delBtn);
        dom.todoListEl.appendChild(row);
    });
}

function startTodoEdit(index) {
    const rows = dom.todoListEl.querySelectorAll('.todo-item');
    const row = rows[index];
    if (!row) return;
    const contentEl = row.querySelector('.todo-content');
    if (!contentEl) return;

    const finish = async (commit) => {
        contentEl.removeEventListener('blur', onBlur);
        contentEl.removeEventListener('keydown', onKeydown);
        if (commit) {
            const text = contentEl.innerText.replace(/\r\n/g, '\n').trim();
            if (text.length > 0) {
                todoItems[index].content = text;
            }
        }
        renderTodoList();
        await saveTodo();
    };
    const onBlur = () => finish(true);
    const onKeydown = (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            contentEl.blur();
        } else if (e.key === 'Escape') {
            contentEl.removeEventListener('blur', onBlur);
            finish(false);
        }
    };

    contentEl.contentEditable = 'true';
    contentEl.classList.add('editing');
    contentEl.focus();
    const range = document.createRange();
    range.selectNodeContents(contentEl);
    range.collapse(false);
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
    contentEl.addEventListener('blur', onBlur);
    contentEl.addEventListener('keydown', onKeydown);
}

export function initTodoEvents() {
    addClick('btn-toggle-todo', () => openTodoModal());
    addClick('btn-close-todo', closeTodoModal);
    addClick('btn-add-todo', () => todoAddItem());
    addClick('btn-delete-all-todo', () => todoDeleteAll());
}
