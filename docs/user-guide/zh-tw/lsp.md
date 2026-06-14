# LSP — Language Server Protocol 支援

> 語言：[English](../en/lsp.md) ｜ **正體中文**

evva 與 Language Server 整合,直接在你的終端機 coding agent session 中提供語意層級的程式碼智慧。

## 它能做什麼

`lsp_request` 工具讓 agent 能向 language server 查詢:

| 操作 | 說明 |
|---|---|
| `go_to_definition` | 跳到符號被定義的位置 |
| `find_references` | 找出某符號的所有使用處 |
| `hover` | 取得某位置的型別資訊與文件 |
| `document_symbols` | 列出一個檔案中的所有符號 |
| `workspace_symbol` | 以名稱在整個專案中搜尋符號 |
| `go_to_implementation` | 找出某介面或型別的實作 |
| `call_hierarchy` | 追蹤呼叫圖(進入/外出的呼叫) |

此外,LSP server 會自動推送**診斷(diagnostics)**(錯誤、警告)——它們會以 system reminder 的形式出現在對話中,agent 不需要主動請求。

---

## 逐步設定(以 Go 為例)

本教學以 Go 與 gopls 為例。同樣的模式適用於 TypeScript、Rust,或任何有 LSP server 的語言。

### 1. 安裝 LSP Server

```bash
go install golang.org/x/tools/gopls@latest
```

確認它在你的 PATH 上:

```bash
which gopls
# /Users/you/go/bin/gopls

gopls version
# golang.org/x/tools/gopls v0.21.1
```

### 2. 在你的專案中啟動 evva

切換到一個 Go 專案(任何有 `go.mod` 檔的目錄)並啟動 evva:

```bash
cd /path/to/your-go-project
evva
```

evva 會自動偵測 `go.mod` 與 PATH 上的 `gopls`——不需要設定檔。

若自動偵測失效(罕見),建立一份最小設定:

```yaml
# .evva/lsp_servers.yml
servers:
  gopls:
    command: gopls
    extensions:
      ".go": "go"
    startupTimeout: "120s"
    maxRestarts: 3
```

### 3. 確認 LSP 正常運作

在 evva session 中,請 agent 使用 LSP:

```
find the definition of the Server type in server.go
```

agent 會以 `operation: "go_to_definition"` 呼叫 `lsp_request`。第一次請求會啟動 gopls(初次索引可能需要 30–60 秒)。後續請求則是即時的。

要手動確認,檢查 daemon 清單:

```
daemon_list
```

你應該會看到一筆 LSP daemon 紀錄:

```
daemon l1 [lsp/running] server=gopls state=running restarts=0/3
```

### 4. 測試常見操作

在 evva 中試這些提示,以操演不同的 LSP 功能:

- **定義(Definition):**「where is `Manager` defined in `pkg/tools/lsp/tool.go`?」
- **參考(References):**「find all references to `Daemon` in the project」
- **Hover:**「what type is `ctx` at line 22 of `tool.go`?」
- **符號(Symbols):**「list all symbols in `agent.go`」
- **工作區搜尋(Workspace search):**「search the workspace for symbols matching 'Agent'」
- **呼叫階層(Call hierarchy):**「show me the call hierarchy for `NewTool`」

---

## 其他語言的設定

### TypeScript / JavaScript

```bash
npm install -g typescript-language-server typescript
```

當 `package.json` 存在且有 `.ts`/`.tsx` 檔時自動偵測。

### Rust

```bash
rustup component add rust-analyzer
```

當 `Cargo.toml` 存在時自動偵測。

### 其他語言

建立 `.evva/lsp_servers.yml`,填入你語言的 server。常見的 server:

| 語言 | Server | 安裝 |
|---|---|---|
| Python | pyright | `pip install pyright` |
| Zig | zls | [zigtools.org/zls](https://zigtools.org/zls/) |
| C/C++ | clangd | `apt install clangd` / `brew install llvm` |

Python 的設定範例:

```yaml
servers:
  pyright:
    command: pyright-langserver
    args: ["--stdio"]
    extensions:
      ".py": "python"
    startupTimeout: "60s"
```

---

## 手動設定參考

在你的專案根目錄建立 `.evva/lsp_servers.yml`(專案層級),或建立 `~/.evva/lsp_servers.yml`(使用者層級,套用到所有專案)。對於同一個 server 名稱,專案層級的設定會覆寫使用者層級。

完整設定格式:

```yaml
servers:
  gopls:
    command: gopls                    # 必填:binary 名稱或路徑
    args: []                          # 選填:CLI 參數
    extensions:                       # 必填:副檔名 → 語言 ID
      ".go": "go"
    env:                              # 選填:環境變數
      GOPATH: "${HOME}/go"
    startupTimeout: "120s"            # 選填:等待初始化的最長時間(預設 30s)
    maxRestarts: 3                    # 選填:崩潰復原上限(預設 3)
```

環境變數展開(`${VAR}`、`${HOME}`)在 `command`、`args` 與 `env` 的值中皆有效。

---

## 用法

`lsp_request` 工具是**延遲載入(deferred)**的——agent 在需要 LSP 能力時會透過 `tool_search` 發現它。你可以這樣問 agent:

- 「Where is `UserService` defined?」
- 「Find all references to `authenticate`」
- 「What's the type of this variable?」
- 「List all symbols in `handler.go`」
- 「Who calls `processRequest`?」

agent 會在適當時機自動使用 `lsp_request`。

---

## 檢查 Server 狀態

LSP server 會以 daemon 的形式註冊進 evva 的 daemon 系統。用 `daemon_list` 查看執行中的 LSP server:

```
daemon l1 [lsp/running] server=gopls state=running restarts=0/3
```

用 `daemon_output l1` 查看該 server 近期的 log 輸出。

---

## 疑難排解

**「gopls not found in PATH」**
安裝缺少的 server(見上方安裝指令)、重啟 evva,再試一次。

**「No LSP server configured for extension .py」**
在 `.evva/lsp_servers.yml` 為你語言的 server 加一筆設定。用錯誤訊息中的 `SuggestServerForExt` 提示來判斷該安裝哪個 server。

**Server 有啟動,但請求回傳空白**
gopls 初次啟動時需要時間索引你的專案。大型專案可能需要 60–120 秒。在設定中調高 `startupTimeout`,並在第一次 `lsp_request` 後稍候——後續請求就會很快。

**診斷沒有出現**
診斷會在某檔案經由 `lsp_request` 開啟後才送達。如果你用 `write`/`edit`/`bash` 編輯了某個檔案,對該檔呼叫 `lsp_request` 以重新整理診斷。

**殘留的 gopls 殭屍程序**
執行 `pkill gopls` 清理。evva 會在關閉時殺掉 server,但若 evva 崩潰,server 程序可能殘留。
