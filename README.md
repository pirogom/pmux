# pmux (Terminal Multiplexer for Windows)

[![Go](https://img.shields.io/badge/Go-1.20+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Wails](https://img.shields.io/badge/Wails-v2-red?style=flat)](https://wails.io)
[![Xterm.js](https://img.shields.io/badge/Xterm.js-Terminal-blue?style=flat)](https://xtermjs.org)
[![Language](https://img.shields.io/badge/Language-English-blue)](#) 
[![Korean](https://img.shields.io/badge/Language-한국어-green)](README.kr.md)

**pmux** is a high-performance terminal multiplexer designed specifically for Windows environments. Powered by Windows Pseudo Console (`ConPTY`), Wails v2, and Xterm.js, **pmux** offers multi-pane split layouts, persistent profile blueprints, real-time multi-client synchronization, and integrated Git repository tracking tailored for **AI Coding Agents**.

---

## 🚀 Key Features

- **Flexible Multi-Pane Terminal Splitting**: Split terminal views horizontally or vertically with mouse-drag split resizing.
- **Real-Time Multi-Client Synchronization**: Synchronize active sessions, pane splits, focus highlights, session renaming, and profile configurations instantly across multiple open GUI windows and web clients via non-blocking WebSocket broadcasts (`/ws/events`).
- **Profile Layout Persistence & Automatic Restoration**: Save custom profiles with command defaults. Pane split layout trees (`SavedLayout`) are automatically saved to `config.json` so launching a profile restores the exact multi-pane layout and working directories.
- **ConPTY Resiliency & TUI Stability**: Uses `coder/websocket` with asynchronous ConPTY resize execution and a 32KB OutPipe buffer, preventing UI freezes during continuous window resizes or running complex TUI applications (Vim, Htop, Neovim, Lazygit, Gitui, etc.).
- **Pane Auto-Reconnect**: Automatically retries WebSocket connections up to 3 times (with a 1-second delay) upon unexpected connection drops, displaying Toast notifications for connection statuses.
- **Collapsible Accordion Sidebar UI**: Collapse or expand the **ACTIVE SESSIONS** and **PROFILES** sidebar sections with persistent `localStorage` states. When PROFILES is collapsed, the ACTIVE SESSIONS list dynamically expands to utilize all remaining vertical space.
- **Integrated Git Dashboard & Uncommitted File Tracker**: Detects the current working directory of the active terminal pane to quickly highlight uncommitted files and repository branch status.

---

## 🤖 AI Agent & Git Workflow Philosophy

> [!NOTE]
> **Git Prerequisites & Purpose**
> - The Git integration requires the `git` executable to be installed and available in your system's `PATH`.
> - **pmux** was originally created to optimize workflows when working alongside **AI Coding Agents** (such as Gemini/Antigravity, Claude, etc.).
> - The built-in Git sidebar is designed primarily to give you a quick visual overview of **uncommitted/staged files** in your current working directory.
> - For actual repository management (committing, pushing, branching, rebasing): Feel free to **command your AI Agent to handle git operations**, or run powerful TUI Git clients directly inside a split terminal pane—such as [`gitui`](https://github.com/extrawurst/gitui) or [`lazygit`](https://github.com/jesseduffield/lazygit).

---

## ⚙️ Command-Line Arguments & Usage

`pmux` supports several command-line flags for controlling the daemon server, checking statuses, or launching client instances.

| Flag / Option | Description | Default |
| :--- | :--- | :--- |
| *(None)* | Launch the GUI client (automatically starts background daemon server if not running) | - |
| `--status` | Check if the background daemon server is currently running | `false` |
| `--stop`, `--kill` | Stop/terminate the running background daemon server | `false` |
| `--server` | Run strictly in background daemon server mode (Headless) | `false` |
| `--port <number>` | Specify the custom listen port for the daemon server | `4799` |
| `--help`, `-h` | Display command-line usage and help details | `false` |

### Command Examples

```powershell
# 1. Launch GUI Client (Default)
pmux.exe

# 2. Check if background daemon server is running
pmux.exe --status

# 3. Stop the running daemon server
pmux.exe --stop

# 4. Run strictly in background daemon server mode on a custom port
pmux.exe --server --port 5000

# 5. Display CLI help details
pmux.exe --help
```

---

## 🛠️ Technology Stack

| Layer | Technologies Used |
| :--- | :--- |
| **Backend & Core** | Go (Golang), Wails v2, `coder/websocket` (v1.8.15), Windows ConPTY API |
| **Frontend UI** | HTML5, Vanilla CSS3 (Custom Theme Design System), JavaScript (ES6+) |
| **Terminal Renderer** | Xterm.js (Fit Addon, Canvas Addon, WebGL Addon) |

---

## 💻 Getting Started

### Prerequisites
- **OS**: Windows 10 / Windows 11 (build 1809 or higher for ConPTY support)
- **Go**: Version 1.20 or higher
- **Wails v2**: CLI installed (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- **Node.js**: Node.js 16+ and `npm`

### Live Development Mode
Run live development mode with hot-reloading for the frontend:

```bash
wails dev
```

### Building for Production
Build a standalone executable package:

```bash
wails build
```
The compiled executable will be located in the `build/bin/` directory.

---

## 📖 Usage Guide

### 1. Splitting Terminals
- Click the **Split Horizontal** (`Ctrl+Shift+D`) or **Split Vertical** (`Ctrl+Shift+E`) buttons on the top workspace action bar to split the currently focused pane.
- Drag the splitter dividers to adjust pane proportions dynamically.

### 2. Profile Management
- Click **➕ Add Profile** in the left sidebar to save reusable environment profiles (Command, Arguments, Working Directory).
- Launching a profile automatically recreates its saved split pane blueprint.

### 3. Sidebar Collapsible Sections
- Click the **ACTIVE SESSIONS** or **PROFILES** section headers to toggle section visibility.
- Collapse **PROFILES** to give **ACTIVE SESSIONS** maximum vertical space for long session lists.

---

## 📄 License

Distributed under the MIT License. See `LICENSE` for details.
