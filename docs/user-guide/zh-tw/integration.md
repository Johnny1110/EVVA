# 如何在你的 Go 專案中整合 EVVA agent?

> 語言：[English](../en/integration.md) ｜ **正體中文**

<br>

---

<br>

evva 的 agent 執行期是可嵌入的。本 repo 以外的 Go 程式可以匯入 `pkg/agent`,組裝出自己的 ReAct agent——搭配自訂的 LLM provider、自訂工具、自訂事件 sink,以及非預設的家目錄——完全不需要 fork evva,也不需要碰 agent loop。

以下所有內容**只**用到 `pkg/*` 的匯入。Go 的 `internal/` 規則會在編譯期強制這一點:下游模組若不小心伸手進 `evva/internal/...`,就無法編譯。

### 1. 把 evva 加進你的模組

```bash
go get github.com/johnny1110/evva
```

### 2. 挑選你的 providers

空白匯入(blank-import)`pkg/llm/builtins`,即可在預設 registry 上註冊 Anthropic / DeepSeek / Ollama;或者你自己註冊一個自訂 provider。

```go
import (
    _ "github.com/johnny1110/evva/pkg/llm/builtins" // anthropic/deepseek/ollama
    "github.com/johnny1110/evva/pkg/llm"
)

// 選用:註冊你自己的 LLM client。
func registerGemini() {
    llm.DefaultRegistry().MustRegister("gemini",
        func(cfg llm.APIConfig, model string, opts ...llm.Option) (llm.Client, error) {
        return newGeminiClient(cfg, model, opts...), nil
    })
}
```

你的 `llm.Client` 實作要滿足六個方法:`Name()`、`Model()`、`SupportsDeferLoading()`、`Complete(ctx, msgs, tools)`、`Stream(ctx, msgs, tools, sink)`,以及 `Apply(opts...)`。`SupportsDeferLoading()` 回報該 provider 是否原生支援 `defer_loading`(不確定就回 `false`——這樣 agent 會讓 tools 陣列保持穩定,以保留 prompt caching)。契約細節見 `pkg/llm/client.go`。

### 3. 用你自己的 AppHome 載入 Config

`config.LoadDefault()` 會以 `~/.evva/` 開機,以便與內建 CLI 相容。下游 app 則建立自己的:

```go
import "github.com/johnny1110/evva/pkg/config"

cfg, err := config.Load(config.LoadOptions{
    AppName: "myapp",
    AppHome: filepath.Join(home, ".myapp"),
})

// 為你挑選的 provider 裝上 API key。
cfg.LLMProviderConfig["anthropic"] = config.APIConfig{
    ApiURL:    "https://api.anthropic.com",
    ApiSecret: os.Getenv("ANTHROPIC_API_KEY"),
}
```

兩個帶有不同 `*config.Config` 指標的 agent 可以在同一個 process 內共存——agent loop 內部沒有全域單例。

### 4. 撰寫自訂工具(選用)

工具要滿足 `pkg/tools.Tool`:`Name()`、`Description()`、`Schema()`,以及 `Execute(ctx, logger, input)`。工廠函式會收到一個 `pkg/tools.State`,讓工具能讀取當前的 `*config.Config` 與 agent 的工作目錄。

```go
import (
    "github.com/johnny1110/evva/pkg/tools"
)

type pingTool struct{}

func (pingTool) Name() string            { return "ping" }
func (pingTool) Description() string     { return "respond with pong" }
func (pingTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (pingTool) Execute(_ context.Context, _ *slog.Logger, _ json.RawMessage) (tools.Result, error) {
    return tools.Result{Content: "pong"}, nil
}
```

### 5. 用你自己的 sink 消費事件

`pkg/event.Sink` 只有一個方法:`Emit(event.Event)`。用 `event.Multi{Sinks: [...]}` 把事件扇出到多個 sink,或用 `event.BubbleUp` 包住上層的 sink 以改寫 `ParentID`。

```go
import "github.com/johnny1110/evva/pkg/event"

type stdoutSink struct{}

func (stdoutSink) Emit(e event.Event) {
    switch e.Kind {
    case event.KindText:
        if e.Text != nil {
            fmt.Println(e.Text.Text)
        }
    case event.KindToolUseStart:
        if e.ToolUseStart != nil {
            fmt.Printf("→ %s\n", e.ToolUseStart.Name)
        }
    }
}
```

### 6. 建立 agent

有兩個建構子。依你想要替你接好多少東西來選。

**`agent.New(Config, ...Option)`——一次到位的路徑。** 一個宣告式的 `Config` 會解析出一個 persona(找不到就退回 `evva`)、自動載入 `EVVA.md` / `USER_PROFILE.md` 記憶與 skill catalog、載入權限 store,並裝上核准/提問 broker。當你想要「電池全包」的體驗(以及內建 TUI)時最適合:

```go
import "github.com/johnny1110/evva/pkg/agent"

ag, _ := agent.New(agent.Config{
    AppConfig:      cfg,
    PermissionMode: "bypass", // "" → YAML → "default"
}, agent.WithSink(stdoutSink{}), agent.WithMaxIterations(20))

resp, _ := ag.Run(context.Background(), "list files under /tmp")
```

若要互動式的終端機 UI,請建立內建的 `pkg/ui/bubbletea` UI、把它當成 sink 傳入,並把 `ag.Controller()` 交給 `tui.Attach`——見下方的 full-host 範例。用 `agent.BuildAgentRegistry` + `reg.Register(agent.AgentDefinition{...})` 註冊你自己的 persona,再傳入 `Config.Personas` + `Config.Persona`。

**`agent.NewWithProfile(profile, ...Option)`——單點選用(à-la-carte)的路徑。** 只接你傳進去的東西。用 `NewProfile` 建一個 profile,再手動加上 config、sink、自訂工具與權限姿態:

```go
prof, _ := agent.NewProfile(
    "myapp",
    "you are a concise assistant",
    []tools.ToolName{tools.READ_FILE, tools.BASH},
    "anthropic", "claude-sonnet-4-6",
    agent.ProfileOptions{},
)

ag, _ := agent.NewWithProfile(prof,
    agent.WithConfig(cfg),
    agent.WithSink(stdoutSink{}),
    agent.WithMaxIterations(20),
    agent.WithHeadlessBypass(), // 或 WithPermissionMode(agent.PermissionBypass)——typed 常數
    agent.WithCustomTool("ping", func(tools.State) (tools.Tool, error) {
        return pingTool{}, nil
    }),
)

resp, err := ag.Run(context.Background(), "list files under /tmp")
```

### 完整可執行範例

兩個可實際執行的下游消費者,兩者皆**零 `internal/*` 匯入**:

- [`examples/full-host/`](../../../examples/full-host/main.go)——標準的 full host:透過一次到位建構子接上內建 TUI + persona + 權限,而且是放在一個**獨立的 Go 模組**裡(因此 Go 的 `internal/` 規則會在編譯器層級強制只用 pkg 的邊界)。
- [`examples/minimal-host/`](../../../examples/minimal-host/main.go)——迷你 host:自訂 LLM provider、自訂工具、自訂 sink,以及一個程式化的 skill,透過 `NewWithProfile` 跑一輪。

```bash
go run ./examples/full-host     # 或 ./examples/minimal-host
```

### 你不能改的東西

以下刻意不屬於公開介面:

- **事件種類(Event kinds)。** `pkg/event.Kind` 常數在你匯入的 evva 版本上是固定的。新增一種 kind 需要改 evva 的程式碼。
- **Agent loop 邏輯。** `LLM 呼叫 → 派發工具 → 收攏結果 → 重複` 這個形狀住在 `internal/agent/loop.go`,不可設定。
- **Sysprompt 內部。** 透過 `NewProfile(..., systemPrompt, ...)` 注入你自己的完整 system prompt;evva 內建的 prompt builder 並未匯出。

如果其中某一項擋住你的使用情境,請 fork 或開 issue。

### 延伸閱讀

- [`docs/contributing/extending.md`](../../../docs/contributing/extending.md)——完整參考,涵蓋每個公開套件、每個擴充點,以及你無法覆寫的部分。
- [`docs/contributing/sdk-stability.md`](../../../docs/contributing/sdk-stability.md)——各套件的穩定性等級,以及如何在 `go.mod` 釘選 evva。
- [`examples/full-host/main.go`](../../../examples/full-host/main.go)——full host(TUI + persona + 權限),獨立模組。
- [`examples/minimal-host/main.go`](../../../examples/minimal-host/main.go)——透過 `NewWithProfile` 的迷你 host。
- [`pkg/agent/downstream_test.go`](../../../pkg/agent/downstream_test.go) + [`converged_downstream_test.go`](../../../pkg/agent/converged_downstream_test.go)——兩個建構子的可複製貼上測試範本。
