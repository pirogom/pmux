# pmux (Windows 전용 터미널 멀티플렉서)

[![Go](https://img.shields.io/badge/Go-1.20+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Wails](https://img.shields.io/badge/Wails-v2-red?style=flat)](https://wails.io)
[![Xterm.js](https://img.shields.io/badge/Xterm.js-Terminal-blue?style=flat)](https://xtermjs.org)
[![English](https://img.shields.io/badge/Language-English-blue)](README.md)
[![Korean](https://img.shields.io/badge/Language-한국어-green)](#)

**pmux**는 Windows 환경에 최적화된 고성능 터미널 멀티플렉서 어플리케이션입니다. Windows Pseudo Console (`ConPTY`), Wails v2 및 Xterm.js를 기반으로 구축되었으며 multi-pane 분할 레이아웃, 프로필 분할 상태 자동 저장/복원, 실시간 다중 클라이언트 동기화 및 **AI 코딩 에이전트 협업 환경**을 위한 Git 대시보드를 제공합니다.

---

## 🚀 주요 기능

- **유연한 Multi-Pane 터미널 분할**: 가로 분할(`Split Horizontal`) 및 세로 분할(`Split Vertical`)과 마우스 드래그 기반의 분할 비율 조절(Resizing) 지원.
- **실시간 다중 클라이언트 동기화 (Multi-Client Real-Time Sync)**: 넌블로킹 웹소켓 브로드캐스트(`/ws/events`)를 기반으로 열려 있는 여러 GUI 창과 웹 클라이언트 간의 세션 생성, Pane 분할/닫기, 포커스 위치, 세션 이름 변경 및 프로필 설정을 **0.001초 실시간 동기화**.
- **프로필 청사진 지속성 및 자동 복원 (Profile Layout Persistence)**: 사용자 정의 프로필(실행 명령어, 인자, 작업 디렉터리) 저장 지원. 세션 분할 이력(`SavedLayout`)이 `config.json`에 자동 저장되어 프로필 클릭 시 이전에 구성된 N개 분할 터미널과 작업 디렉터리가 완벽하게 자동 복원.
- **ConPTY 안정성 및 TUI 먹통 방지**: `coder/websocket` 및 비동기 ConPTY 리사이즈 처리, 32KB OutPipe 버퍼링을 통해 지속적인 창 크기 변경이나 TUI 어플리케이션(Vim, Htop, Neovim, Lazygit, Gitui 등) 실행 시 발생하는 터미널 먹통 및 데드락 현상 방지.
- **Pane 웹소켓 자동 재접속 (Auto-Reconnect)**: 터미널 커넥션이 끊어질 경우 1초 간격으로 최대 3회 자동 재접속 시도 및 Toast 토스트 알림 안내.
- **접고 펼치는 사이드바 아코디언 UI (Collapsible Accordion)**: 사이드바의 **ACTIVE SESSIONS** 및 **PROFILES** 섹션을 접고 펼칠 수 있으며 `localStorage`에 상태가 보존됨. PROFILES 섹션을 접으면 ACTIVE SESSIONS 목록이 남은 수직 공간을 100% 가득 활용하는 유연한 높이 동적 확장 기능 제공.
- **통합 Git 대시보드 및 커밋되지 않은 파일 감지**: 현재 활성화된 터미널 Pane의 작업 디렉터리를 자동 감지하여 브랜치 상태와 커밋되지 않은 파일 유무를 빠르게 시각적으로 확인.

---

## 🤖 AI 에이전트 협업 및 Git 사용 안내

> [!NOTE]
> **Git 요구사항 및 개발 철학**
> - Git 관련 기능은 시스템 환경 변수에 `git` 커맨드라인 도구가 설치되어 있어야 정상 작동합니다.
> - **pmux**는 **AI Agent (AI 코딩 에이전트)**와의 자유로운 협업 및 개발 환경 제공을 목적으로 제작되었습니다.
> - 내장된 Git 사이드바 기능은 **현재 폴더에 커밋되지 않은 변경 사항(Uncommitted files)이 있는지 빠르게 확인하는 목적**으로 존재합니다.
> - 커밋, 푸시 등 본격적인 저장소 관리 및 제어 작업은 **AI Agent를 부려먹으시거나**, 분할 터미널(Pane) 내에서 [`gitui`](https://github.com/extrawurst/gitui)나 [`lazygit`](https://github.com/jesseduffield/lazygit) 같은 TUI 클라이언트를 띄워서 편하게 관리하시기 바랍니다!

---

## ⚙️ 커맨드라인 실행 인자 (CLI Arguments & Usage)

`pmux`는 데몬 서버 제어, 상태 확인 및 클라이언트 구동을 위한 커맨드라인 실행 플래그를 지원합니다.

| 실행 인자 / 옵션 | 설명 | 기본값 |
| :--- | :--- | :--- |
| *(인자 없음)* | GUI 클라이언트 실행 (백그라운드 데몬 서버가 없는 경우 자동 시작) | - |
| `--status` | 백그라운드 데몬 서버의 구동 여부 및 포트 확인 | `false` |
| `--stop`, `--kill` | 구동 중인 백그라운드 데몬 서버 정지 | `false` |
| `--server` | 백그라운드 데몬 서버 전용 모드로 실행 (Headless) | `false` |
| `--port <number>` | 백그라운드 데몬 서버의 리슨 포트 지정 | `4799` |
| `--help`, `-h` | 명령어 사용법 및 도움말 출력 | `false` |

### 실행 명령어 예시

```powershell
# 1. 일반 GUI 클라이언트 실행 (기본)
pmux.exe

# 2. 백그라운드 데몬 서버 구동 상태 확인
pmux.exe --status

# 3. 구동 중인 데몬 서버 정지
pmux.exe --stop

# 4. 사용자 정의 포트로 데몬 서버만 전용 백그라운드 실행
pmux.exe --server --port 5000

# 5. 커맨드라인 도움말 출력
pmux.exe --help
```

---

## 🛠️ 기술 스택 (Tech Stack)

| 레이어 | 사용 기술 |
| :--- | :--- |
| **백엔드 & 코어** | Go (Golang), Wails v2, `coder/websocket` (v1.8.15), Windows ConPTY API |
| **프론트엔드 UI** | HTML5, Vanilla CSS3 (Custom Theme Design System), JavaScript (ES6+) |
| **터미널 렌더러** | Xterm.js (Fit Addon, Canvas Addon, WebGL Addon) |

---

## 💻 시작하기 (Getting Started)

### 사전 요구사항
- **OS**: Windows 10 / Windows 11 (ConPTY 지원을 위한 Build 1809 이상)
- **Go**: Version 1.20 이상
- **Wails v2**: CLI 설치 (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)
- **Node.js**: Node.js 16+ 및 `npm`

### 개발 모드 실행 (Live Development)
프론트엔드 핫 리로딩(Hot-Reloading)을 포함한 개발 모드 실행:

```bash
wails dev
```

### 프로덕션 빌드 (Production Build)
독립 실행형 바이너리 패키지 빌드:

```bash
wails build
```
컴파일된 실행 파일은 `build/bin/` 디렉터리에 생성됩니다.

---

## 📖 사용 가이드

### 1. 터미널 분할 및 닫기 (Splitting & Closing Terminals)
- 상단 워크스페이스 툴바의 **Split Horizontal** (`Ctrl+Shift+D`) 또는 **Split Vertical** (`Ctrl+Shift+E`) 버튼을 클릭하여 현재 포커스된 Pane을 분할합니다.
- 스플리터 구분선을 마우스로 드래그하여 분할 비율을 자유롭게 조절할 수 있습니다.
- **Pane 닫기**: `Ctrl` 키를 누르면 각 Pane의 상단 오른쪽 모서리에 **✖ (닫기)** 버튼이 표시되며, 이 ✖ 버튼을 클릭하여 해당 Pane을 닫을 수 있습니다.

### 2. 프로필 관리 (Profile Management)
- 좌측 사이드바의 **➕ Add Profile** 버튼을 눌러 자주 사용하는 환경 프로필(명령어, 인자, 작업 디렉터리)을 등록합니다.
- 프로필 클릭 시 이전에 저장된 분할 청사진 레이아웃대로 세션이 즉시 열립니다.

### 3. 사이드바 접기/펼치기 (Collapsible Sidebar)
- **ACTIVE SESSIONS** 또는 **PROFILES** 섹션 헤더를 클릭하여 접거나 펼칠 수 있습니다.
- **PROFILES**를 접으면 **ACTIVE SESSIONS** 영역이 높이를 가득 채워 긴 세션 목록도 시원하게 확인할 수 있습니다.

---

## 🔌 ConPTY 호스트 (conpty.dll / OpenConsole.exe)

pmux는 ConPTY 세션을 만들 때 OS에 내장된 `conhost.exe`를 그대로 쓰지 않습니다. 대신 Microsoft가 공식적으로 재배포 가능한 형태로 제공하는 ConPTY 호스트 — `conpty.dll` + `OpenConsole.exe` (Windows Terminal이 자체적으로 번들하는 것과 동일한 소스, MIT 라이선스) — 를 `go:embed`로 `pmux.exe` 바이너리 안에 직접 내장해서 사용합니다. 이는 실제로 발견된 호환성 버그를 우회하기 위한 조치입니다: MSYS2 `-defterm` 셸(예: `msys2_shell.cmd -defterm -here -no-start -mingw64`)에서 포그라운드 프로세스를 Ctrl+C로 중단할 때 ConPTY 연결이 끊기면서 pane이 강제로 종료되는 현상이 있었는데, 이는 일부 버전의 시스템 `conhost.exe`에서만 발생하고 Windows Terminal이 쓰는 `OpenConsole.exe` 빌드에서는 재현되지 않았습니다.

- 원본 바이너리는 `resources/conpty/x64/`, `resources/conpty/x86/`에 있고, 빌드 태그(`resources/conpty/embed_amd64.go`, `embed_386.go`)로 아키텍처별로 분리되어 embed되므로 실제 빌드에는 자기 아키텍처에 맞는 쌍만 포함됩니다.
- 최초 실행 시 이 두 파일을 `%USERPROFILE%\.pmux\bin\64\`(386 빌드는 `\32\`)에 추출하고(embed된 내용의 해시가 다르면 자동 갱신), 그 경로의 `conpty.dll`을 로드합니다 — 별도 설치 과정 없이 `pmux.exe` 단독 실행 파일만으로 동작합니다. 만약 `pmux.exe`와 같은 폴더에 이미 `conpty.dll`이 놓여있다면(수동 배치 등) 그쪽을 우선 사용합니다. 둘 다 사용할 수 없으면 기존처럼 OS의 `kernel32.dll`/시스템 `conhost.exe`로 자동 폴백합니다.

### ConPTY 호스트 버전 갱신 방법

Microsoft가 중요한 수정사항을 배포하면 공식 NuGet 패키지에서 받아 번들 파일을 교체하면 됩니다. **`conpty.dll`과 `OpenConsole.exe`는 반드시 짝이 맞는 버전으로 함께 교체해야 합니다** — 버전이 어긋나면 "No process is on the other end of the pipe" 같은 크래시가 발생하는 것으로 알려져 있습니다.

```powershell
# 1. 사용 가능한 버전 확인
curl -s https://api.nuget.org/v3-flatcontainer/microsoft.windows.console.conpty/index.json

# 2. 원하는 버전 다운로드 (버전 문자열 수정)
curl -sL -o conpty.nupkg "https://www.nuget.org/api/v2/package/Microsoft.Windows.Console.ConPTY/<version>"

# 3. nupkg는 zip 파일이므로 압축 해제
Expand-Archive conpty.nupkg -DestinationPath conpty_pkg

# 4. 아키텍처별로 매칭되는 쌍을 기존 파일에 덮어쓰기
#    x64: runtimes\win-x64\native\conpty.dll  ->  resources\conpty\x64\conpty.dll
#         build\native\runtimes\x64\OpenConsole.exe -> resources\conpty\x64\OpenConsole.exe
#    x86: runtimes\win-x86\native\conpty.dll  ->  resources\conpty\x86\conpty.dll
#         build\native\runtimes\x86\OpenConsole.exe -> resources\conpty\x86\OpenConsole.exe
```

파일 교체 후 다시 빌드(`wails build` 또는 `go build`)하면 됩니다. 다음 실행 시 pmux가 embed된 내용의 해시 변경을 자동으로 감지해서 `%USERPROFILE%\.pmux\bin\`을 알아서 갱신하므로 수동 정리는 필요 없습니다.

---

## 📄 라이선스 (License)

본 프로젝트는 MIT 라이선스에 따라 배포됩니다. 자세한 내용은 `LICENSE` 파일을 참조하세요.
