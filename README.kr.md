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
- **프로필 폴더 및 드래그&드롭 정리 (Profile Folders & Drag-and-Drop)**: PROFILES 사이드바에서 **📁** 버튼으로 폴더(트리 루트 아이템)를 생성하고, 프로필을 폴더 위에 드래그&드롭하여 서브 아이템으로 이동할 수 있습니다. 프로필/폴더 순서를 자유롭게 변경하고, 폴더 이름을 더블클릭으로 바로 수정할 수 있으며, 폴더 삭제 시 내부 프로필은 자동으로 루트 목록으로 이동합니다.
- **ConPTY 안정성 및 TUI 먹통 방지**: `coder/websocket` 및 비동기 ConPTY 리사이즈 처리, 32KB OutPipe 버퍼링을 통해 지속적인 창 크기 변경이나 TUI 어플리케이션(Vim, Htop, Neovim, Lazygit, Gitui 등) 실행 시 발생하는 터미널 먹통 및 데드락 현상 방지.
- **Pane 웹소켓 자동 재접속 (Auto-Reconnect)**: 터미널 커넥션이 끊어질 경우 1초 간격으로 최대 3회 자동 재접속 시도 및 Toast 토스트 알림 안내.
- **접고 펼치는 사이드바 아코디언 UI (Collapsible Accordion)**: 사이드바의 **ACTIVE SESSIONS** 및 **PROFILES** 섹션을 접고 펼칠 수 있으며 `localStorage`에 상태가 보존됨. PROFILES 섹션을 접으면 ACTIVE SESSIONS 목록이 남은 수직 공간을 100% 가득 활용하는 유연한 높이 동적 확장 기능 제공.
- **통합 Git 대시보드 (순수 Go, 시스템 git 불필요)**: 현재 활성화된 터미널 Pane의 작업 디렉터리를 자동 감지하여 브랜치 상태, 커밋되지 않은 파일(스테이징 상태 구분), 커밋 로그를 표시하고, 스테이징/언스테이징, 커밋, Fetch/Pull/Push, 브랜치 체크아웃, 파일별 Diff 미리보기를 지원합니다. Diff 미리보기와 커밋 상세는 **별도 팝업 창**으로 열리며 **줄 단위 문법 하이라이트**(추가/삭제/헌크 라인 색상 구분)가 적용됩니다.
- **SSH 주소록 및 원클릭 접속 (SSH Address Book & Quick Connect)**: Pane별 확장 툴바(`Ctrl` 키를 누르면 표시)에서 설정된 SSH 클라이언트로 원클릭 SSH 접속이 가능합니다. 암호화된 주소록(`~/.pmux/ssh.conf`)을 제공하며, 다른 기기로의 데이터 이전을 위한 비밀번호 보호 Export/Import를 지원합니다.
- **작업 폴더별 노트 (Quill 리치 텍스트 편집기)**: 상단 툴바의 **📝 Notes** 버튼으로 현재 활성 Pane의 작업 디렉터리에 종속된 메모를 작성할 수 있습니다. 작업 폴더별로 여러 개의 노트를 관리하며(`~/.pmux/note/`에 JSON 저장), 기본 노트가 항상 하나 존재합니다. Quill 기반 WYSIWYG 툴바로 폰트 크기, 헤딩, Bold/Italic/Underline/취소선, 텍스트/배경 색상, 순서형/불릿 목록, 정렬, 인용구, 코드 블록, 링크, 서식 제거를 지원하며, 입력 중 자동 저장됩니다.

---

## 🤖 AI 에이전트 협업 및 Git 사용 안내

> [!NOTE]
> **Git 요구사항 및 개발 철학**
> - 내장 Git 기능은 **순수 Go([go-git](https://github.com/go-git/go-git)) 기반**으로 동작합니다. 시스템에 `git` 커맨드라인 도구가 설치되어 있지 않아도 사용할 수 있습니다.
> - Git 사이드바에서 **변경 사항 확인, 스테이징/언스테이징, 커밋, Fetch/Pull/Push, 커밋 로그, 브랜치 조회/체크아웃, 파일별 Diff 미리보기**를 모두 지원합니다.
> - Diff 미리보기와 커밋 상세는 **팝업 뷰어**(✖ 버튼, `Esc`, 또는 배경 클릭으로 닫기)로 표시되며 **줄 단위 하이라이트**가 적용됩니다 — 추가된 줄, 삭제된 줄, 헌크 헤더, 파일 메타데이터가 색상으로 구분됩니다.
> - History 목록에서 **커밋 해시**를 클릭하면 해당 커밋의 전체 메타데이터(해시, 작성자, 날짜, 메시지)와 전체 Diff가 팝업에 함께 표시됩니다.
> - **Refresh Interval** 선택기(1~10초)로 Git 패널의 자동 새로고침 주기를 조절할 수 있으며, 선택한 값은 **config에 저장되어 재시작 시 복원**되고 **연결된 모든 클라이언트에 실시간 동기화**됩니다.
> - push/pull 인증은 시스템에 이미 존재하는 인증 정보를 자동으로 재사용합니다.
>   - **SSH 원격**: OpenSSH Authentication Agent(`ssh-add`로 등록된 키) → `~/.ssh/id_ed25519`/`id_rsa`/`id_ecdsa` 개인키 (known_hosts 검증 포함)
>   - **HTTPS 원격**: Windows 자격 증명 관리자(git.exe/Git Credential Manager가 저장한 자격 증명) → `~/.git-credentials` 파일
> - 인증 정보를 찾지 못하면 패널에 원인과 해결 방법을 안내합니다.
> - 참고: go-git 특성상 submodule 내부 변경 감지와 Git LFS 지원은 제한적입니다.

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
| `--log` | 콘솔로의 디버그/상태 로그 출력 활성화 | `false` |
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

# 5. 데몬 서버 실행 시 콘솔 로그 출력 활성화 (디버깅용)
pmux.exe --server --log

# 6. 커맨드라인 도움말 출력
pmux.exe --help
```

---

## 💡 SSH 클라이언트 권장 사항 (Windows OpenSSH vs MSYS2 SSH)

> [!WARNING]
> **MSYS2 내장 SSH 클라이언트의 Ctrl+C 프로세스 종료 문제**
>
> MSYS2 / Cygwin 계열에 내장된 `ssh` (`/usr/bin/ssh` 등)는 POSIX PTY 에뮬레이션 신호 처리 방식과 Windows ConPTY 간의 호환성 문제로 인해, SSH로 원격 서버에 접속한 상태에서 **`Ctrl + C` (`^C`, SIGINT)**를 입력하면 원격지의 실행 중인 명령뿐만 아니라 **로컬 `ssh` 프로세스 자체까지 즉시 kill(비정상 종료)**되어 세션이 끊어지는 현상이 발생합니다.

### 권장 해결 방법: Windows 기본 OpenSSH 사용
SSH 원격 접속 시에는 MSYS2 내장 `ssh` 대신 **Windows 기본 내장 OpenSSH 클라이언트**를 사용할 것을 강력히 권장합니다.

- **실행 파일 경로**: `C:\Windows\System32\OpenSSH\ssh.exe`
- Windows 기본 `ssh.exe`는 Windows ConPTY API와 완벽하게 호환되어, 원격 서버에서 `Ctrl + C`를 누르더라도 SSH 세션 연결이 유지되고 원격 프로세스만 정상적으로 인터럽트됩니다.
- 프로필(Profile) 등록 시 명령어로 `C:\Windows\System32\OpenSSH\ssh.exe`를 지정하거나, Windows `PATH` 환경 변수에서 `C:\Windows\System32\OpenSSH`가 MSYS2 경로보다 우선순위에 오도록 설정하여 사용하세요.

---

## 🔐 SSH 관리자 (SSH Manager & 주소록)

내장 SSH 관리자를 사용하면 `ssh` 명령을 직접 입력하지 않고도 어떤 Pane에서든 SSH 접속을 시작할 수 있습니다.

### SSH 관리자 열기
- `Ctrl` 키를 누르고 있으면 각 Pane의 **왼쪽 상단**에 확장 툴바가 표시됩니다 (오른쪽 상단에는 ✖ 닫기 버튼이 함께 표시됨).
- 툴바의 **SSH 아이콘**을 클릭하면 해당 Pane에 대한 SSH 관리자가 열립니다.

### SSH 클라이언트 경로
- **SSH Client Path** 필드에서 SSH 클라이언트 실행 파일 경로를 지정합니다 (**📁 Browse** 버튼으로 파일 선택 가능).
- 기본값: `C:\Windows\System32\OpenSSH\ssh.exe` (Windows 10/11에 기본 내장된 OpenSSH 클라이언트).
- 기본 클라이언트를 사용하면 터미널에 `ssh ...`만 입력됩니다 (System32는 항상 `PATH`에 포함됨). 커스텀 경로를 설정하면 전체 경로를 따옴표로 감싸 입력하며, PowerShell Pane에서는 `& ` 프리픽스가 붙습니다.

### 주소록
각 항목은 다음 정보를 저장합니다:
- **Name** – 항목을 구분하기 위한 닉네임/타이틀.
- **Description** – 부가 설명 (옵션).
- **Host Address** – 접속할 대상 서버 주소 (IP, 호스트명, URL; `host:port` 형식 지원).
- **Account** – 로그인 계정 (옵션). 설정 시 `ssh user@host`, 미설정 시 `ssh host`가 입력됩니다.

### 접속 (Connect)
- 주소를 선택하고 **Connect**를 클릭합니다.
- pmux는 선택한 Pane의 터미널에 `ssh` 명령을 입력해줍니다 — 이후의 호스트 키 확인, 비밀번호 입력 등은 터미널에서 직접 처리하면 됩니다.

### 데이터 저장 및 보안
- 설정과 주소록은 `~/.pmux/ssh.conf`에 저장됩니다.
- 파일은 **Windows DPAPI**(`CryptProtectData`)로 암호화되어, **동일한 Windows 사용자 계정 + 동일한 기기**에서만 복호화할 수 있습니다 — 소스코드에 암호화 키가 존재하지 않습니다.
- 파일에 **버전 값**이 포함되어 있어, 호환되지 않는 pmux 버전이 작성한 파일은 읽지 않고 덮어쓰지도 않습니다.
- > [!NOTE]
  > Windows 관리자가 계정 비밀번호를 **강제 초기화**하면 DPAPI로 보호된 데이터를 복구하지 못할 수 있습니다. 일반적인 비밀번호 변경은 문제없습니다.

### Export / Import (다른 기기로 데이터 이전)
`ssh.conf`는 Windows 사용자 계정과 기기에 바인딩되어 있으므로, 다른 컴퓨터로의 이전은 비밀번호 보호 Export/Import로 진행합니다:
- **Export** – 비밀번호를 입력(확인 입력 포함)합니다. 주소록이 입력한 비밀번호로 암호화(scrypt 키 파생 + AES-256-GCM)되어 `*.pmuxssh` 파일로 저장됩니다.
- **Import** – `*.pmuxssh` 파일을 선택하고 해당 비밀번호를 입력합니다. **비밀번호가 일치해야만** Import가 성공하며, 틀린 비밀번호는 거부됩니다.
- Import 시 현재 주소록은 가져온 데이터로 **교체**됩니다.

---

## 🛠️ 기술 스택 (Tech Stack)

| 레이어 | 사용 기술 |
| :--- | :--- |
| **백엔드 & 코어** | Go (Golang), Wails v2, `coder/websocket` (v1.8.15), Windows ConPTY API |
| **프론트엔드 UI** | HTML5, Vanilla CSS3 (Custom Theme Design System), JavaScript (ES6+), Quill v2 (리치 텍스트 편집기) |
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
- 상단 워크스페이스 툴바의 **Split Horizontal** 또는 **Split Vertical** 버튼을 클릭하여 현재 포커스된 Pane을 분할합니다.
- 스플리터 구분선을 마우스로 드래그하여 분할 비율을 자유롭게 조절할 수 있습니다.
- **Pane 닫기**: `Ctrl` 키를 누르면 각 Pane의 상단 오른쪽 모서리에 **✖ (닫기)** 버튼이 표시되며, 이 ✖ 버튼을 클릭하여 해당 Pane을 닫을 수 있습니다.

### 2. 프로필 관리 (Profile Management)
- 좌측 사이드바의 **➕ Add Profile** 버튼을 눌러 자주 사용하는 환경 프로필(명령어, 인자, 작업 디렉터리)을 등록합니다.
- 프로필 클릭 시 이전에 저장된 분할 청사진 레이아웃대로 세션이 즉시 열립니다.
- **프로필 폴더**: **📁 Add Folder** 버튼으로 폴더(트리 루트 아이템)를 생성하면 즉시 이름 편집 모드가 시작됩니다. 프로필을 폴더 위에 드래그&드롭하면 폴더의 서브 아이템이 되고, 프로필/폴더를 드래그하여 순서를 자유롭게 바꿀 수 있습니다. 목록 하단 빈 영역에 드롭하면 프로필이 루트(폴더 밖)로 이동합니다. 폴더 이름은 **더블클릭**으로 바로 수정할 수 있으며(`Enter` 저장 / `Esc` 취소), 폴더 삭제 시 내부 프로필은 모두 루트 목록으로 자동 이동됩니다.

### 3. 사이드바 접기/펼치기 (Collapsible Sidebar)
- **ACTIVE SESSIONS** 또는 **PROFILES** 섹션 헤더를 클릭하여 접거나 펼칠 수 있습니다.
- **PROFILES**를 접으면 **ACTIVE SESSIONS** 영역이 높이를 가득 채워 긴 세션 목록도 시원하게 확인할 수 있습니다.

### 4. 클립보드 복사/붙여넣기 및 터미널 제어 문자 (`^V`)
- **복사 (Copy)**: 텍스트를 드래그하여 선택한 상태에서 `Ctrl + C`를 누르면 클립보드에 복사됩니다. (선택 영역이 없을 때 `Ctrl + C`는 터미널 프로세스 인터럽트 `^C`로 동작합니다.)
- **붙여넣기 (Paste)**: `Ctrl + V` 또는 `Shift + Ctrl + V`를 누르면 클립보드 내용이 터미널에 붙여넣어집니다.
- **제어 문자 `^V` 전송 (Vim Visual Block / Bash Literal Next)**:
  - Windows의 기본 붙여넣기 단축키와의 충돌을 피하기 위해, 터미널 제어 문자 **`^V` (`ASCII 0x16`)**는 **`Alt + V`**로 매핑되어 있습니다.
  - Vim에서 세로 블록 선택(Visual Block) 모드로 진입하거나, Bash에서 특수 키 이스케이프(Literal Next)를 입력할 때는 **`Alt + V`**를 누르면 됩니다.

### 5. 노트 (작업 폴더별 메모)
- 상단 툴바의 **📝 Notes** 버튼을 클릭하면 활성 Pane의 작업 디렉터리에 대한 노트 팝업이 열립니다.
- 노트는 **작업 디렉터리별로 저장**됩니다 — 각 작업 폴더는 고유한 노트 목록을 가지며 `~/.pmux/note/<hash>.json`에 저장됩니다. 노트가 없는 작업 폴더에서 Notes를 열면 기본 노트가 자동 생성되고, 모든 노트를 삭제해도 빈 노트 1개가 항상 남습니다.
- **➕ New Note** 버튼으로 노트를 추가하고, 왼쪽 목록에서 클릭하여 전환할 수 있으며, 제목 필드에서 노트 제목을 수정할 수 있습니다. 노트 위에 마우스를 올린 뒤 **✖** 버튼으로 삭제합니다.
- 툴바로 텍스트를 서식 지정합니다: 폰트 크기, 헤딩(H1~H6), Bold/Italic/Underline/취소선, 텍스트/배경 색상, 순서형/불릿 목록, 정렬, 인용구, 코드 블록, 링크, 서식 제거.
- 입력을 멈추면 약 0.5초 후 자동 저장되며, 팝업을 **✖** 버튼으로 닫을 때도 저장됩니다. 편집기에서도 텍스트 선택·복사가 정상 동작합니다.

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
