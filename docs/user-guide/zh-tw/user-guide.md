# EVVAgent — 使用手冊

## 目錄

- [1. 總覽 — TUI 介面一覽](#1-總覽--tui-介面一覽)
- [2. Slash 指令](#2-slash-指令)
  - [/config — 即時設定](#config--即時設定)
  - [/model — 切換提供者/模型](#model--切換提供者模型)
- [3. 快捷鍵](#3-快捷鍵)
- [4. Yank 模式 — 從對話紀錄複製](#4-yank-模式--從對話紀錄複製)
- [5. 對話紀錄搜尋](#5-對話紀錄搜尋)
- [6. 權限系統](#6-權限系統)
  - [權限模式](#權限模式)
  - [核准提示](#核准提示)
  - [權限規則](#權限規則)
- [7. 子代理](#7-子代理)
- [8. 設定參考](#8-設定參考)
  - [evva-config.yml](#evva-configyml)
  - [.env（選用）](#env選用)
  - [CLI 參數](#cli-參數)
- [9. 執行模式 — TUI vs CLI](#9-執行模式--tui-vs-cli)
- [10. 日誌](#10-日誌)

---

## 1. 總覽 — TUI 介面一覽

```
┌──────────────────────────────────────────────────────────────┐
│ banner box / transcript                                      │
│                                                              │
│  ▶ user prompt                                               │
│  assistant text…                                             │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│ ▰ TASKS         (only when non-empty)                        │
│   ▶ wire migration                                           │
├──────────────────────────────────────────────────────────────┤
│ ‹⠹ explorer› ‹▶ writer› ‹✔ reviewer›   ← active sub-agents   │
├──────────────────────────────────────────────────────────────┤
│ overlay panels: /config · /model · approval · suggestions    │
├──────────────────────────────────────────────────────────────┤
│ > input                                                      │
├──────────────────────────────────────────────────────────────┤
│ ‹⠋ RUN› ◆ evva ◆ ▸ model ◆ in N out M ◆ CTX ▰▰▱…▱ 12%       │
└──────────────────────────────────────────────────────────────┘
```

面板在空白時會折疊至零高度。狀態列始終顯示在底部。

---

## 2. Slash 指令

在輸入框開頭輸入 `/`，畫面會顯示建議面板。隨著你輸入更多字元，列表會依大小寫不敏感的 prefix 比對進行過濾。當輸入內容與某個指令**完全相符**時，該列會變為綠色並顯示 `✓`——按下 Enter 即可執行。

| 按鍵 | 效果 |
| --- | --- |
| `Tab` | 自動補全為高亮的建議選項 |
| `↑` / `↓` | 移動高亮建議選項 |
| `Enter` | 送出當前輸入（若為有效指令則執行） |
| `Esc` | 在此輸入階段關閉建議面板 |

可用指令：

| 指令 | 功能 |
| --- | --- |
| `/config` | 開啟設定表單 |
| `/model` | 切換 LLM 提供者/模型 — **會清除對話歷史** |
| `/clear` | 清除對話紀錄（保留 banner） |
| `/exit`、`/quit` | 離開 |

### /config — 即時設定

開啟一個帶邊框的表單，列出所有可編輯的設定：

```
┌─ /CONFIG ────────────────────────────────────────┐
│ ▶ max_iterations           30                    │
│   max_tokens               4096                  │
│   auto_compact_threshold   0.8                   │
│   display_thinking         true                  │
│   fetch_max_bytes          100000                │
│   tavily_api_key           ****wxyz              │
│   anthropic.api_key        (empty)               │
│   …                                              │
│ [↑↓] navigate · [Enter] edit/toggle · [Esc] close│
└──────────────────────────────────────────────────┘
```

| 按鍵 | 效果 |
| --- | --- |
| `↑` / `↓` | 移動游標 |
| `Enter` | 編輯聚焦的欄位（布林值直接切換） |
| `Enter`（編輯器中） | 套用並儲存 |
| `Esc` | 取消編輯（或在列表模式關閉面板） |

API 金鑰欄位會開啟密碼遮罩編輯器；貼上功能照常運作（顯示維持遮罩狀態）。

**即時生效**（立即套用）：

- `max_iterations` — 迴圈安全上限
- `display_thinking` — 切換對話紀錄中的思考區塊顯示
- `auto_compact_threshold` — 上下文壓縮的觸發時機

**已儲存但需重新啟動**（需要重建 client / web 工具）：

- `max_tokens`、`fetch_max_bytes`、`tavily_api_key`、所有 `<provider>.api_key`、所有 `<provider>.api_url`

每次編輯都會立即寫入 `~/.evva/config/evva-config.yml`。

### /model — 切換提供者/模型

開啟一個清單，顯示程式已知的所有 `(provider, model)` 組合，游標預設停在目前使用中的項目上：

```
┌─ /MODEL ─────────────────────────────────────────────────────┐
│ Swapping clears the conversation — provider-specific state   │
│ (thinking signatures) can't carry across providers.          │
│                                                              │
│   ollama / qwen3.6                                           │
│   anthropic / claude-sonnet-4-6                              │
│   anthropic / claude-opus-4-7                                │
│ ▶ deepseek / deepseek-v4-pro  (current)                      │
│   deepseek / deepseek-v4-flash                               │
│   openai / gpt-5.5                                           │
│                                                              │
│ [↑↓] navigate · [Enter] switch · [Esc] cancel                │
└──────────────────────────────────────────────────────────────┘
```

| 按鍵 | 效果 |
| --- | --- |
| `↑` / `↓` | 瀏覽清單 |
| `Enter` | 切換至高亮的模型 |
| `Esc` | 取消 |

**重要：** 切換模型必定會清除對話。Anthropic 的 `ThinkingSignature` 綁定特定提供者——若帶著舊對話紀錄跨提供者切換，下一次請求會回傳 400 錯誤。新的選擇也會儲存為 `default_provider` + `default_model`，讓下次啟動時直接沿用。

若有執行中的任務則無法切換；請先按 Esc 取消任務，再輸入 `/model`。

---

## 3. 快捷鍵

| 按鍵 | 效果 |
| --- | --- |
| `Enter` | 送出 |
| `Ctrl+J` / `Alt+Enter` | 插入換行（多行輸入） |
| `↑` / `↓` | 瀏覽提示歷史（輸入框為空或已在瀏覽時） |
| `Esc` | 取消執行中的任務 / 關閉面板 |
| `Ctrl+C` | 按一次：取消執行中任務 · 閒置時：離開 |
| `Ctrl+D` | 離開（輸入框為空時） |
| `Ctrl+O` | 切換展開所有工具結果（折疊/展開較長的 bash 與 read 輸出） |
| `Ctrl+Y` | 開啟 **yank 模式** — 選取區塊並複製其乾淨內容 |
| `Ctrl+F` | 開啟 **對話紀錄搜尋** — 輸入查詢字串，`Enter`/`n` 循環跳轉 |
| `Shift+Tab` | 循環切換 **權限模式** — `default → accept_edits → plan → bypass → …` |
| `PgUp` / `PgDown` / `Home` / `End` | 捲動對話紀錄 |
| 滑鼠滾輪 | 捲動對話紀錄 |

---

## 4. Yank 模式 — 從對話紀錄複製

對話紀錄中的每個區塊都會在左側繪製時間軸裝飾線（`│`、`├─` 等），讓對話以結構化方式呈現。缺點是：一般終端機的拖曳選取會複製畫面上所有可見內容——包含這些裝飾符號。貼到其他視窗後會得到像這樣的結果：

```
▶ who are you?
│
│ I'm evva — an interactive coding assistant…
│
```

要複製不含裝飾的乾淨內容，evva 內建了 **yank 模式**，能夠辨識區塊邊界。這是標準的乾淨複製途徑；在終端機不完整支援剪貼簿逸出序列時，也是唯一可用的方式。

**使用 `Ctrl+Y` 開啟。** 一次只會在一個區塊上顯示青色粗體的邊欄提示；狀態列上方的提示文字會顯示當前游標位置（`yank 3/5`）與按鍵對照。

| 按鍵 | 效果 |
| --- | --- |
| `j` / `↓` | 下一個區塊（較新） |
| `k` / `↑` | 上一個區塊（較舊） |
| `g` | 跳到第一個區塊 |
| `G` | 跳到最後一個區塊 |
| `Enter` / `c` | 將聚焦區塊的乾淨文字複製到系統剪貼簿 |
| `e` | 僅切換此區塊的展開/折疊（在複製長工具輸出前很實用） |
| `q` / `Esc` | 離開 yank 模式（清除邊欄提示） |
| `Ctrl+C` | 離開 + 退出 evva |

**複製了什麼。** 每個區塊提供一個 `PlainText()` 視圖，會移除 ANSI 控制碼與裝飾符號。使用者提示區塊對應提示文字，助手文字區塊對應 markdown 原始碼（非渲染後輸出），工具區塊則為呼叫標頭（`◢ name(...)`）加上結果內文。成功時狀態列會閃爍 `copied N chars`。

**技術細節 — OSC52。** Yank 模式使用 [OSC52](https://wezfurlong.org/wezterm/escape-sequences.html#operating-system-command-sequences) 終端機逸出序列將內容寫入剪貼簿。不需外部函式庫，也不依賴 `pbcopy`。終端機會將逸出序列轉發至作業系統剪貼簿。

| 終端機 | 是否預設可用？ |
| --- | --- |
| **iTerm2** | 是（預設） |
| **kitty** | 是 |
| **WezTerm** | 是 |
| **Alacritty** | 是 |
| **Ghostty** | 是 |
| **Apple Terminal.app** | 預設不可用 — 需啟用 `編輯 → 允許剪貼簿存取` 或更換終端機 |
| **tmux** | 需設定 `set -g set-clipboard on` |
| **GNU screen** | 大多無法使用；請改用 `Ctrl+Y` 從宿主終端機操作 |

若寫入失敗（內容超過 100 KB、終端機阻擋），狀態列會顯示 `clipboard: <error>`，yank 模式保持開啟，讓你可以嘗試其他區塊。

**為什麼不用原生拖曳選取？** evva 啟用滑鼠捕捉是為了讓滾輪能夠捲動對話紀錄。這項取捨使得拖放複製無法以原生方式運作——即使現代終端機支援 `Shift`/`Alt`+拖曳的繞過機制，選取結果仍然包含渲染後的裝飾符號（因為它們本就是畫在螢幕上的內容）。Yank 模式是將乾淨內容從程式內帶出的正式流程。

---

## 5. 對話紀錄搜尋

按下 `Ctrl+F` 開啟搜尋列。輸入查詢字串後按 `Enter` 跳到第一個匹配項。按 `n` 向前循環匹配項，或按 `N`（Shift+n）向後循環。按 `Esc` 關閉搜尋列。

---

## 6. 權限系統

### 權限模式

evva 透過**權限模式**對每個工具呼叫進行把關。共有四種模式，使用 `Shift+Tab` 循環切換：

| 模式 | 不需詢問即自動允許 | 適合情境 |
| --- | --- | --- |
| **`default`** | 唯讀工具（`read`、`tree`、`grep`、`glob`、`web_*`、`json_query`、`calc`）、代理自協調工具（`agent`、`task_*`、`skill`、`tool_search`、`ask_user_question`），以及**唯讀 bash 指令**（`ls`、`cat`、`head`、`grep`、`git status`、`git log`、…）。檔案寫入與其他 bash 指令**會詢問**。 | 初學者、敏感工作、預設姿態 |
| **`accept_edits`** | 同 `default` + 檔案編輯（`edit`、`write`、`notebook_edit`）+ 常見檔案系統 bash 指令（`mkdir`、`touch`、`mv`、`cp`、`rmdir`、`ln`、`chmod`、`chown`）。 | 審閱中的程式碼迭代 |
| **`plan`** | 與 `default` 相同的唯讀安全清單。清單外的任何操作**直接拒絕**（不顯示提示）。 | 在決定修改前先探索程式碼庫 |
| **`bypass`** | 全部允許。危險指令分類仍會在背景記錄，但絕不阻擋。 | **僅限隔離容器與虛擬機使用** — 會傳遞至子代理 |

當前模式在狀態列中以彩色標籤顯示（`⛨ plan`、`⛨ bypass`、…）。`default` 會折疊此欄位以保持介面簡潔。

**以指定模式啟動：**

```bash
evva -permission-mode=plan                # 最安全：先調查
evva -permission-mode=accept_edits        # 自動套用編輯 + 安全的檔案系統指令
evva -permission-mode=bypass              # 無提示；僅限沙箱環境
```

CLI 參數優先；持久性預設值可寫入 `evva-config.yml`：

```yaml
permission_mode: default     # default | accept_edits | plan | bypass
```

### 核准提示

在 `default` / `accept_edits` / `plan` 模式下，任何需要核准的操作都會彈出模態對話框：

```
┌─ APPROVAL ─────────────────────────────────────────┐
│ tool: bash                                         │
│ mode: default  risk: dangerous (sudo)              │
│ reason: matches dangerous prefix                   │
│                                                    │
│ input: sudo rm /tmp/evil-file                      │
│                                                    │
│ ▶ [1] Allow once                                   │
│   [2] Allow for this session                       │
│   [3] Deny                                         │
│                                                    │
│ [↑↓] choose · [Enter] confirm · [Esc] deny         │
└────────────────────────────────────────────────────┘
```

| 按鍵 | 效果 |
| --- | --- |
| `↑` / `↓` | 在按鈕間移動 |
| `1` / `a` | 允許一次 — 僅執行本次呼叫 |
| `2` / `s` | 允許此工作階段 — 同時新增記憶體規則，後續類似呼叫不再提示 |
| `3` / `d` | 拒絕 — 再按 Enter 可輸入提供給模型的拒絕原因 |
| `Enter` | 確認高亮選項（或送出拒絕原因） |
| `Esc` | 等同拒絕 |
| `Ctrl+C` | 拒絕 + 退出 |

**「允許此工作階段」** 會根據呼叫內容選擇合適的規則形式：對 `bash` 儲存第一個 token（因此核准 `git status` 後，後續 `git …` 呼叫都會放行，而非任意指令）；對 `read`/`write`/`edit` 儲存檔案路徑；其他工具則為工具層級的放行。工作階段規則在退出後消失；若要持久化，請手動編輯 `permissions.json`。

平行核准（代理在同一回合發出兩個 `bash` 呼叫）會堆疊 — 處理完最上層後，下一個會自動浮現。

### 權限規則

規則讓核准持久化，跨執行不會重複看到相同提示。有兩個作用範圍：

- `<workdir>/.evva/permissions.json` — **專案級**：跟隨 repo，可透過 git 分享
- `~/.evva/permissions.json` — **使用者級**：在所有工作目錄生效

格式：

```json
{
  "permissions": {
    "allow": [
      "bash(git:*)",
      "bash(npm:*)",
      "read(src/**)",
      "edit",
      "tree"
    ],
    "deny": [
      "bash(sudo:*)",
      "bash(rm -rf /)"
    ],
    "ask": [
      "bash(npm publish)"
    ]
  }
}
```

**規則語法**：`ToolName` 匹配該工具的所有呼叫。`ToolName(content)` 加入內容匹配：

| 工具 | 內容語法 | 範例 |
| --- | --- | --- |
| `bash` | `prefix:*`、`pattern *`、`git *` 或精確指令 | `bash(git:*)`、`bash(npm install *)`、`bash(make build)` |
| `read`、`write`、`edit`、`notebook_edit` | 針對 `file_path` 的 doublestar glob | `read(src/**)`、`write(./tmp/*.txt)`、`edit(**/*.go)` |
| 其他 | 對原始輸入的精確字串比對 | 少用；建議使用工具層級規則 |

**優先順序：**

1. `bypass` 模式 — 一律允許，忽略規則。
2. **deny 規則** — 最先檢查，在所有非 bypass 模式中優先於 allow。
3. **ask 規則** — 強制顯示提示，即使有更廣泛的 allow（或模式安全清單）匹配。
4. `plan` 模式 + 工具不在唯讀安全清單 → **拒絕**（無提示）。
5. 唯讀 / 自協調安全清單 → 允許。
6. Bash + 分類器判定為唯讀（`ls`、`cat`、`git status`、…）→ 允許。
7. 僅 `accept_edits`：`edit`/`write`/`notebook_edit` → 允許；bash 常見檔案系統指令（`mkdir`/`mv`/`cp`/…）→ 允許。
8. **allow 規則** — 匹配 → 執行。
9. 最終回退 — 詢問。

各行為（deny/ask/allow）內的來源優先順序為 `session > project > user`，因此工作階段的「允許此工作階段」會覆蓋使用者範圍規則，但永遠不會覆蓋 deny。

---

## 7. 子代理

根代理可以生成子代理（`explore` 為唯讀檢查、`general-purpose` 則具備寫入能力）。執行中的子代理會在輸入框上方以水平橫列晶片顯示。非同步子代理在背景完成——其摘要會在下一次迭代中以模擬使用者訊息的形式出現在頂端，對話會自動接收。

你不需要手動驅動子代理；模型會自行決定何時生成。設計上為兩層架構（子代理無法再生成子代理）。

---

## 8. 設定參考

### evva-config.yml

路徑：`~/.evva/config/evva-config.yml`。首次啟動時自動建立。可透過 TUI 的 `/config` 即時編輯，或手動修改：

```yaml
# Agent loop
max_iterations: 30
max_tokens: 4096
auto_compact_threshold: 0.8
display_thinking: true

# Default model used at startup (overwritten by /model swap)
default_provider: deepseek
default_model: deepseek-v4-pro

# Permission stance at startup. Cycle at runtime with Shift+Tab; -permission-mode CLI flag overrides.
permission_mode: default     # default | accept_edits | plan | bypass

# Web tooling
fetch_max_bytes: 100000
tavily_api_key: ""

# Per-provider credentials. Empty api_url falls back to the constant's default.
providers:
  anthropic: { api_key: "", api_url: "" }
  deepseek:  { api_key: "", api_url: "" }
  openai:    { api_key: "", api_url: "" }
  ollama:    { api_url: "" }
```

### .env（選用）

放置於工作目錄或 `~/.evva/.env`。僅用於部署 / 日誌控制——絕非使用者偏好設定：

```bash
APP_ENV=dev            # dev | prod
LOG_LEVEL=info         # debug | info | warn | error
LOG_FORMAT=text        # text | json
LOG_DIR=               # 留空 → stdout；填寫路徑 → 將日誌寫入該目錄
SKILLS_DIR=skills      # ~/.evva/ 下的子路徑
USER_PROFILE=user_profile.md
```

### CLI 參數

```bash
evva                                # 互動式 TUI（stdout 為 TTY 時預設）
evva -temp 0.7                      # 取樣溫度（預設不設定）
evva -max-tokens 2048               # 每次 completion 輸出上限（覆蓋 YAML）
evva -max-iters 40                  # 迴圈迭代上限（覆蓋 YAML）
evva -permission-mode=plan          # 以 plan 模式啟動（唯讀）
evva -permission-mode=bypass        # 以關閉權限閘門啟動
evva -no-tui "explain loop.go"      # 單次純文字模式
echo "list files in /tmp" | evva -no-tui   # 管線輸入提示
```

---

## 9. 執行模式 — TUI vs CLI

**互動式 TUI**（stdout 為 TTY 時預設）。包含對話紀錄、面板、狀態列等完整功能。

**純文字 CLI**（`-no-tui`，或 stdout 被管線重定向時）。單次流程：從參數/stdin 讀取提示 → 執行代理 → 以純文字串流事件 → 退出。CLI 模式沒有互動式核准介面——任何需要提示的呼叫都會**自動拒絕**，並提示可傳入 `-permission-mode=bypass` 或在 `permissions.json` 中新增規則。適用於腳本與 CI 環境。

---

## 10. 日誌

每個代理的 JSON 日誌預設存放於 `log/<agent-id>/<agent-id>.log`。可在 `.env` 中設定 `LOG_DIR` 來重新導向，或保留空白以同時輸出至 stdout。`LOG_LEVEL=debug` 會揭露每次迭代的 `turn.start` / `llm.call` / `tool.dispatch` / `tool.result` 行——在除錯代理卡住或無限迴圈時非常實用。
