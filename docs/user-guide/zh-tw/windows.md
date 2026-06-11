# 在 Windows 上使用 evva

自 v1.7.0 起，evva 提供原生 Windows 執行檔（windows/amd64 為正式支援層級、
windows/arm64 為盡力支援）。本頁只說明與 macOS/Linux 不同之處；未提及的
功能行為皆相同。

## 安裝

1. 從 [最新 release](https://github.com/johnny1110/evva/releases/latest)
   下載 `evva-windows-amd64.zip`（或 `-arm64`）。
2. 將 `evva.exe` 解壓到 `PATH` 上的目錄
   （例如 `%LOCALAPPDATA%\Programs\evva\`）。
3. 在 **Windows Terminal** 執行 `evva`。

也可以用 Go 1.25+ 安裝：`go install github.com/johnny1110/evva/cmd/evva@latest`
（會放在 `%USERPROFILE%\go\bin`）。

若 SmartScreen 攔下下載的執行檔：右鍵 → 內容 → **解除封鎖**。
目前的 release 執行檔尚未簽章。

`evva update` 與其他平台用法相同。Windows 上唯一的差異是機制性的、
使用者無感：執行中的 exe 會先被改名為 `evva.exe.old`，新版本接替原位；
殘留的 `.old` 會在下次啟動 evva 時自動清掉。

## 前置需求

| 項目 | 原因 |
| --- | --- |
| [Git for Windows](https://gitforwindows.org) | agent 的 `bash` 工具、`monitor` daemon 與 lifecycle hooks 都透過它附帶的 **Git Bash** 執行。evva 會先從 `git` 的安裝位置推導 `bash.exe`，再找常見安裝目錄，最後才看 `PATH`（絕不使用 `System32\bash.exe` —— 那是 WSL 啟動器）。 |
| Windows Terminal（建議） | TUI 在舊版 conhost 也能跑（Windows 10 1903+ 支援 VT），但只在 Windows Terminal 上驗證過。 |
| Python 和/或 Node（選用） | 供 `repl` 工具使用。Windows 上 Python 的尋找順序是 `py` 啟動器 → `python` → `python3`。 |

沒裝 Git Bash 時 evva 仍可啟動並顯示警告；檔案工具、web 工具、LSP、MCP
照常運作，但 `bash`/`monitor`/hooks 會回傳明確錯誤，直到安裝 Git for
Windows 為止。

**`EVVA_SHELL`** 可在任何平台覆寫 shell 自動偵測——指向任何接受
`-c` 參數的 POSIX shell 即可（例如 MSYS2 的 `bash.exe`）。

## Shell 方言

`bash` 工具在 Windows 上一樣說 **POSIX bash**——這正是要求 Git Bash 的
意義。在 shell 內磁碟路徑會以 POSIX 形式呈現（`C:\Users\me` 變成
`/c/Users/me`）；git 與多數工具兩種形式都接受。刻意不提供
PowerShell/cmd 模式。

## Windows 上的 `evva service`

`start` / `stop` / `status` 都可用：daemon 會脫離終端機、把
pidfile/token/addr 寫到 `%USERPROFILE%\.evva\service\`，`stop` 會終止它
（硬殺——service 本身是 crash-safe 設計，重啟時會還原 sessions、mail、
membership、alarms）。

`evva service install-unit` **尚無 Windows 範本**（目前只有 launchd 和
systemd）。要開機自動啟動，請在工作排程器建立登入時執行
`evva service start --foreground` 的工作。

## 已知限制（v1.7）

- Release 執行檔未簽章（首次執行會遇到 SmartScreen 提示）。
- windows/arm64 為交叉編譯，未在實機驗證。
- 沒有 service 自動啟動範本（見上節）。
- 程序樹終止使用 `taskkill /T`；fork 速度快過快照的病態指令理論上可能
  留下孤兒程序——遇到請回報。
