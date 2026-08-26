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
- **Profile Folders & Drag-and-Drop Organization**: Organize profiles into folders right from the PROFILES sidebar tree — create folders with the **📁** button, drag profiles onto folders to make them sub-items, reorder profiles and folders freely, double-click a folder name to rename it inline, and delete folders with the profiles inside automatically moved back to the root list.
- **ConPTY Resiliency & TUI Stability**: Uses `coder/websocket` with asynchronous ConPTY resize execution and a 32KB OutPipe buffer, preventing UI freezes during continuous window resizes or running complex TUI applications (Vim, Htop, Neovim, Lazygit, Gitui, etc.).
- **Pane Auto-Reconnect**: Automatically retries WebSocket connections up to 3 times (with a 1-second delay) upon unexpected connection drops, displaying Toast notifications for connection statuses.
- **Collapsible Accordion Sidebar UI**: Collapse or expand the **ACTIVE SESSIONS** and **PROFILES** sidebar sections with persistent `localStorage` states. When PROFILES is collapsed, the ACTIVE SESSIONS list dynamically expands to utilize all remaining vertical space.
- **Integrated Git Dashboard (pure Go, no system git required)**: Detects the working directory of the active terminal pane and shows branch status, uncommitted files (with staging state), and commit history — with staging/unstaging, commits, Fetch/Pull/Push, branch switching, and per-file diff previews. Diff previews and commit details open in a **separate popup window** with **line-level syntax highlighting** (added/removed/hunk lines are color-coded).
- **SSH Address Book & Quick Connect**: A per-pane extension toolbar (visible while holding `Ctrl`) provides one-click SSH connections through your configured SSH client. The encrypted address book (stored in `~/.pmux/ssh.conf`) supports password-protected Export/Import for migrating data between machines.

---

## 🤖 AI Agent & Git Workflow Philosophy

> [!NOTE]
> **Git Prerequisites & Purpose**
> - The built-in Git integration is **pure Go ([go-git](https://github.com/go-git/go-git)) based** — no system `git` executable is required.
> - The Git sidebar supports **change inspection, staging/unstaging, commits, Fetch/Pull/Push, commit history, branch switching, and per-file diff previews**.
> - Diff previews and commit details are rendered in a **popup viewer** (close with `✖`, `Esc`, or by clicking the backdrop) with **line-level syntax highlighting** — added lines, removed lines, hunk headers, and file metadata are color-coded for readability.
> - Clicking a **commit hash** in the History list opens the commit detail popup showing the full commit metadata (hash, author, date, message) together with its complete diff.
> - The **Refresh Interval** selector (1–10s) controls how often the git panel auto-refreshes; the chosen value is **persisted** in the config and restored on the next launch, and is **synchronized across all connected clients** in real time.
> - push/pull reuses credentials already present on your system: **SSH remotes** use the OpenSSH Authentication Agent or `~/.ssh` keys (with `known_hosts` verification); **HTTPS remotes** use Windows Credential Manager (Git Credential Manager) or a `~/.git-credentials` file. When no credentials are found, the panel explains how to fix it.
> - Known limitation: submodule change detection and Git LFS support are limited in go-git.

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
| `--log` | Enable debug and status logging output to console | `false` |
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

# 5. Start daemon server with console logging enabled (for debugging)
pmux.exe --server --log

# 6. Display CLI help details
pmux.exe --help
```

---

## 💡 Recommended SSH Client (Windows OpenSSH vs MSYS2 SSH)

> [!WARNING]
> **MSYS2 SSH Client Forced Exit on Ctrl+C Issue**
>
> The built-in `ssh` binary from MSYS2 / Cygwin environments (`/usr/bin/ssh`) has known signal handling conflicts between its POSIX PTY emulation layer and Windows ConPTY. When pressing **`Ctrl + C` (`^C`, SIGINT)** inside an active remote SSH session, the local `ssh` process itself receives the termination signal and gets **killed immediately**, abruptly closing the remote connection.

### Recommended Solution: Use Native Windows OpenSSH
For SSH connections, we strongly recommend using the **native Windows OpenSSH client**:

- **Executable Path**: `C:\Windows\System32\OpenSSH\ssh.exe`
- The native Windows `ssh.exe` is fully compatible with Windows ConPTY. Pressing `Ctrl + C` correctly interrupts the running remote foreground process without killing the local SSH client.
- When creating a Profile, set the command to `C:\Windows\System32\OpenSSH\ssh.exe` or ensure `C:\Windows\System32\OpenSSH` has higher precedence than MSYS2 paths in your system's `PATH` environment variable.

---

## 🔐 SSH Manager (Address Book & Quick Connect)

The built-in SSH Manager lets you start SSH connections from any pane without typing the `ssh` command manually.

### Opening the SSH Manager
- Press and hold the `Ctrl` key to reveal the extension toolbar at the **top-left corner** of each pane (the ✖ close button appears at the top-right).
- Click the **SSH icon** in the toolbar to open the SSH Manager for that pane.

### SSH Client Path
- Set the SSH client executable path in the **SSH Client Path** field (use the **📁 Browse** button to pick a file).
- Default: `C:\Windows\System32\OpenSSH\ssh.exe` (the OpenSSH client bundled with Windows 10/11).
- When the default client is used, pmux types `ssh ...` into the terminal (System32 is always on `PATH`). For a custom path, the full quoted path is typed instead (prefixed with `& ` for PowerShell panes).

### Address Book
Each entry stores:
- **Name** – nickname/title for the entry.
- **Description** – optional additional notes.
- **Host Address** – target server address (IP, hostname, or URL; `host:port` syntax is supported).
- **Account** – optional login user. If set, pmux types `ssh user@host`; otherwise it types `ssh host`.

### Connecting
- Select an address and click **Connect**.
- pmux types the `ssh` command into the selected pane's terminal — everything after that (host key confirmation, password entry, etc.) is handled by you in the terminal.

### Data Storage & Security
- Settings and the address book are saved to `~/.pmux/ssh.conf`.
- The file is encrypted with **Windows DPAPI** (`CryptProtectData`), so the data can only be decrypted by the **same Windows user account on the same machine** — no encryption key exists in the source code.
- The file contains a **version field**; if a file was written by an incompatible pmux version, it is not loaded and is never overwritten.
- > [!NOTE]
  > A Windows admin **force-resetting** your account password can make DPAPI-protected data unrecoverable. Normal password changes are fine.

### Export / Import (migrating to another machine)
Because `ssh.conf` is bound to your Windows user account and machine, moving data to another computer is done via password-protected export/import:
- **Export** – enter a password (and confirm it). The address book is encrypted with your password (scrypt key derivation + AES-256-GCM) and saved as a `*.pmuxssh` file.
- **Import** – pick a `*.pmuxssh` file and enter its password. Import succeeds only when the password matches; a wrong password is rejected.
- On import, the current address book is **replaced** by the imported data.

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

### 1. Splitting & Closing Terminals
- Click the **Split Horizontal** or **Split Vertical** buttons on the top workspace action bar to split the currently focused pane.
- Drag the splitter dividers to adjust pane proportions dynamically.
- **Closing Panes**: Press and hold the `Ctrl` key to reveal the **✖ (Close)** button at the top-right corner of each pane, then click the ✖ button to close that pane.

### 2. Profile Management
- Click **➕ Add Profile** in the left sidebar to save reusable environment profiles (Command, Arguments, Working Directory).
- Launching a profile automatically recreates its saved split pane blueprint.
- **Profile Folders**: Click the **📁 Add Folder** button to create a folder (tree root item) and enter inline rename mode immediately. Drag a profile onto a folder to move it inside, or drag profiles/folders to reorder them — dropping onto the empty area below the list moves items back to the root level. Double-click a folder name to rename it inline (`Enter` to confirm, `Esc` to cancel). Deleting a folder moves its profiles back to the root list automatically.

### 3. Sidebar Collapsible Sections
- Click the **ACTIVE SESSIONS** or **PROFILES** section headers to toggle section visibility.
- Collapse **PROFILES** to give **ACTIVE SESSIONS** maximum vertical space for long session lists.

### 4. Clipboard Copy/Paste & Terminal Control Character (`^V`)
- **Copy**: Select text and press `Ctrl + C` to copy to clipboard. (When no text is selected, `Ctrl + C` sends standard SIGINT `^C` to the active process).
- **Paste**: Press `Ctrl + V` or `Shift + Ctrl + V` to paste clipboard contents into the terminal.
- **Send `^V` Control Character (Vim Visual Block / Bash Literal Next)**:
  - To prevent conflicts with Windows clipboard paste shortcuts, the terminal control character **`^V` (`ASCII 0x16`)** is mapped to **`Alt + V`**.
  - In Vim, press **`Alt + V`** to enter Visual Block mode, or in Bash to input literal control characters (Literal Next).

---

## 🔌 ConPTY Host (conpty.dll / OpenConsole.exe)

pmux does **not** rely on the OS's built-in `conhost.exe` for its ConPTY sessions. Instead it embeds Microsoft's official redistributable ConPTY host — `conpty.dll` + `OpenConsole.exe` (same source as Windows Terminal's own bundled host, MIT licensed) — directly into the `pmux.exe` binary via `go:embed`. This works around a real compatibility bug where MSYS2 `-defterm` shells (e.g. `msys2_shell.cmd -defterm -here -no-start -mingw64`) could lose their ConPTY connection and get force-closed when interrupting a foreground process with Ctrl+C — a bug present in some versions of the system `conhost.exe` but not in Windows Terminal's `OpenConsole.exe` build.

- Source binaries live under `resources/conpty/x64/` and `resources/conpty/x86/`, embedded per-architecture via build tags (`resources/conpty/embed_amd64.go`, `embed_386.go`) so a given build only carries the pair matching its own architecture.
- On first use, pmux extracts (and, if the embedded content hash differs, refreshes) these two files to `%USERPROFILE%\.pmux\bin\64\` (or `\32\` for a 386 build) and loads `conpty.dll` from there — no installer step required, `pmux.exe` works standalone. If a `conpty.dll` happens to already sit next to `pmux.exe` (e.g. manual placement), that takes priority instead. If neither is usable, pmux transparently falls back to the OS's `kernel32.dll`/system `conhost.exe`.

### Updating to a newer ConPTY host version

If Microsoft ships an important fix, update the bundled pair from the official NuGet package — **`conpty.dll` and `OpenConsole.exe` must always be replaced together as a matched version pair**; mixing versions is known to cause "No process is on the other end of the pipe"-style crashes.

```powershell
# 1. Check available versions
curl -s https://api.nuget.org/v3-flatcontainer/microsoft.windows.console.conpty/index.json

# 2. Download a specific version (adjust the version string)
curl -sL -o conpty.nupkg "https://www.nuget.org/api/v2/package/Microsoft.Windows.Console.ConPTY/<version>"

# 3. It's a zip — extract it
Expand-Archive conpty.nupkg -DestinationPath conpty_pkg

# 4. Copy the matched pair over the existing files, per architecture
#    x64: runtimes\win-x64\native\conpty.dll  ->  resources\conpty\x64\conpty.dll
#         build\native\runtimes\x64\OpenConsole.exe -> resources\conpty\x64\OpenConsole.exe
#    x86: runtimes\win-x86\native\conpty.dll  ->  resources\conpty\x86\conpty.dll
#         build\native\runtimes\x86\OpenConsole.exe -> resources\conpty\x86\OpenConsole.exe
```

After replacing the files, rebuild (`wails build` or `go build`). On the next run, pmux detects the embedded content hash changed and automatically refreshes `%USERPROFILE%\.pmux\bin\` — no manual cleanup needed.

---

## 📄 License

Distributed under the MIT License. See `LICENSE` for details.
