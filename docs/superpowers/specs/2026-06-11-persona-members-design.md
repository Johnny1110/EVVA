# Persona members — 讓 main-tier 人格以本尊身分加入 swarm（RP-29）

> 日期：2026-06-11 ｜ 狀態：設計定案 ｜ wave：第六波開場，認領 minor **v1.7**
> 分支：`feature/persona-members`（自 `dev`）

## 1. 目標

evva 的核心理念是「one runtime, many personas」。Veronica swarm 目前的成員一律來自
swarm workdir 的 `agents/{main,sub}/<name>/` 磁碟目錄——人設、工具、skills 都是為該
swarm 手寫的。本功能讓 **persona registry 裡的 main-tier 人格**（內建 `evva`，或
`<EVVA_HOME>/agents/` 的使用者自製人格）**以本尊身分**加入任何 swarm、擔任 leader 或
worker：

- 帶入人格的**完整能力**：它自己的 system prompt（執行期組裝，含 conduct 章節）、
  完整 active/deferred 工具組、自己安裝的 skills、workdir `EVVA.md` 專案簡報。
- 疊加 swarm 的 teamwork 協議（身分 grounding、通訊協議、團隊/角色協議、記憶協議）
  與角色對應的 swarm 工具（`send_message`、task 面板、`alarm_set`…）。
- 人格定義**單一事實源**：prompt/工具/skills 永遠與 solo 同源，不做磁碟快照、不分岔。

## 2. 現況（這條縫為什麼存在）

- `swarm.constructMember` 本來就用 `agent.New(Config{Persona: name, Personas: sp.reg})`
  建構成員——與 solo 完全同一條解析路徑（`ResolveMainProfile`）。
- space registry 由 `agent.BuildAgentRegistry(cfg.AppHome)` 建立，**已內含**內建
  `evva` 與 EVVA_HOME 的人格；swarm 成員 def 是事後 `Register` 蓋上去的。
- `resolveMainProfileWithExtra` 對 `"evva"` 有特例：直通 `mainProfile`（全套工具 +
  內部組裝 prompt），**無視** registry 裡同名 def 的 prompt body。
- 目前唯一擋路的是 manifest 載入：`Loader.BuildAll` 強制讀
  `agents/{main,sub}/<name>/system_prompt.md`，目錄不存在即失敗。

結論：功能 = 給 manifest 一個「引用 registry 人格」的成員形態 + 給 prompt 組裝一個
「在內部組裝的 prompt 後附加 swarm 協議」的縫。

## 3. 設計總覽（資料流）

```
evva-swarm.yml ──LoadManifest──► Member{Agent:"evva", FromPersona:true, Model/Effort/WhenToUse/Schedule…}
        │
        ▼ BuildAll（persona 成員跳過磁碟讀取，合成 Loaded{FromPersona:true}）
NewSpace.registerDef ──► sp.reg.Get("evva") 取本尊 def
        │                 + LongRunning/AdvertiseSkills/ensureMain
        │                 + WhenToUse/Model（manifest 覆寫）
        │                 + PromptSuffix = swarm 協議（身分+通訊+團隊+角色+記憶）
        │                 → Register 回 space 私有 registry
        ▼
constructMember ──► agent.New(Persona:"evva")
        │            └► resolveMainProfile 特例 → mainProfile：
        │                 ctx.OmitDate=LongRunning、剝離 solo 排程工具、
        │                 prompt = buildMainPrompt(ctx) + "\n\n" + PromptSuffix
        ▼
活成員：swarm 工具（ToolSet 按角色注入）+ mailbox + roster + 成員記憶目錄 + 合併 skills
```

## 4. Manifest schema（`agentdef/manifest.go`）

成員項（leader 與 workers 皆適用）：

```yaml
leader:
  agent: coordinator          # 既有形態：workdir agents/main/coordinator/
workers:
  - agent: backend-dev        # 既有形態不變
  - persona: evva             # 新形態：引用 registry 的 main-tier 人格
    model: deepseek-v4-pro    # 新欄位（選用）：模型釘選
    effort: ultra             # 新欄位（選用）：low|medium|high|ultra
    when_to_use: "團隊工程師"  # 新欄位（選用）：roster 顯示的專長描述
    schedule: { cron: "...", prompt: "..." }   # 既有欄位照常
    budget_tokens: -1                          # 既有欄位照常
    permission_mode: bypass                    # 既有欄位照常
```

語義與驗證（全部在 `LoadManifest` fail-fast，沿用 schedule/permission_mode 先例）：

1. `agent:` / `persona:` **恰好一個**；兩者皆空或皆填 → 整份 manifest 拒絕。
2. persona 成員的**成員名 = 人格名**（roster 身分即人格名；不支援同人格多實例，
   沿用「no replicas」決策 ⑦）。名字唯一性檢查涵蓋兩種形態。
3. `model:` / `effort:` / `when_to_use:` 對**兩種形態都開放**：
   - persona 成員沒有 profile.yml，這是它唯一的釘選處；
   - dir 成員若在 manifest 設了，**manifest 權威**蓋過 profile.yml（與 schedule
     的 RP-7 §3.7 先例一致：全隊配置收在一個版本化檔案）。
   - `effort` 枚舉驗證；`model` 不對內建表驗證（SDK 自訂模型先例，壞值在 LLM
     client 建構時大聲失敗）。
4. persona 名是否存在 registry **不在 manifest 層驗證**（manifest 解析不持有
   registry）；在 space 組裝時驗證（§6），錯誤訊息點名該成員。

Go 形態：`Member` 增 `FromPersona bool`、`Model string`、`Effort string`、
`WhenToUse string`（名字仍放 `Member.Agent`，下游 budgets/validate/WriteManifest
不用改鍵）。`WriteManifest` 完整 round-trip 新欄位（persona 成員輸出 `persona:`
鍵、不輸出 `agent:`）——RP-8 動態成員 + 重啟重建的前提。

## 5. Loader（`agentdef/loader.go`）

`BuildAll` 對 `FromPersona` 成員**跳過** `Build`（不碰磁碟、不要求目錄存在），合成：

```go
Loaded{
  Def: agent.AgentDefinition{Name: m.Agent, WhenToUse: m.WhenToUse, Model: m.Model},
  FromPersona: true,
  Role: role, Schedule: m.Schedule, Effort: m.Effort, PermissionMode: m.PermissionMode,
  Skills: skill.NewRegistry(),   // 佔位；實際合併在 constructMember（§6）
}
```

dir 成員行為不變，僅追加：manifest 的 `Model`/`Effort`/`WhenToUse` 非空時覆寫
profile.yml 對應值（schedule 既有的覆寫迴圈旁邊加三行）。

## 6. Space 組裝（`internal/swarm/space.go`）

**`registerDef` 改為回傳 error**（呼叫者：`NewSpace` 迴圈與 `AddMember`）。

persona 成員路徑：

```go
base, ok := sp.reg.Get(name)            // BuildAgentRegistry 已含內建+EVVA_HOME 人格
if !ok            → error `persona %q not found in registry`
if !base.IsMain() → error `persona %q is not a main-tier persona`
def := base
def.As = ensureMain(def.As); def.LongRunning = true; def.AdvertiseSkills = true
if ld.Def.WhenToUse != "" { def.WhenToUse = ld.Def.WhenToUse }
if ld.Def.Model    != "" { def.Model    = ld.Def.Model }
def.PromptSuffix = teamProtocolSuffix(name, sp.Name, ld.Role, canWriteMemory)
sp.reg.Register(def)                    // space 私有 registry，solo 不受影響
```

- `teamProtocolSuffix` 由 `teamprompt.go` 的 `injectTeamProtocol` 重構抽出：
  **同一組章節**（swarmIdentity + communicationProtocol + teamProtocolCommon +
  leader/workerProtocol + memoryProtocol），只是不含 persona body。dir 成員維持
  現行「協議拼進 body」行為，零回歸。
- `canWriteMemory`：內建 `evva`（Main kit 必含 write/edit）→ true；磁碟人格 →
  檢查其 `ActiveTools` 含 `write`/`edit`（現行邏輯）。
- **roster 的 `WhenToUse` 用組裝後的 def**（manifest 覆寫要反映在 `list_members`）。

`constructMember` 對 persona 成員的差異只有 skills（§8）；model/effort/permission
釘選、成員記憶目錄 `MkdirAll`、`BindMemberContext`、swarm ToolSet 注入、per-member
permissions.json 全部沿用現行碼（`ld.Def.Model`/`ld.Effort` 已載 manifest 值）。

space 記錄 `personaMembers map[string]bool`（供 §8 的 reload 與未來查詢用）。

## 7. Prompt 組裝（`PromptSuffix` 欄位鏈 + profiles.go）

新欄位 `PromptSuffix string`，三處同步 + round-trip：

| 位置 | 角色 |
| --- | --- |
| `internal/agent/sysprompt.AgentDefinition` | prompt 組裝讀取點 |
| `internal/agent.AgentSpec` | closure-free 轉換層（`DefinitionFromSpec`/`SpecFromDefinition` 對映） |
| `pkg/agent.AgentDefinition`（persona.go） | 公開 DTO（SDK additive 變更） |

組裝規則：

- `mainProfile`（內建 `evva` 路徑）：`resolveMainProfileWithExtra` 的 `"evva"` 分支
  改傳 def 進來（新內部變體 `mainProfileForDef`）——
  `ctx.OmitDate = def.LongRunning`（RP-5 cache 穩定）、套 §7.1 工具剝離、
  最終 `prompt = buildMainPrompt(ctx) + "\n\n" + def.PromptSuffix`（suffix 非空時）。
  `Main()` 公開建構子與 reg==nil fallback 路徑行為不變（solo 零影響）。
- `mainProfileFromDiskAgent`（磁碟人格路徑）：尾端附加 `def.PromptSuffix`（同樣
  非空才加）。這讓 EVVA_HOME 自製人格加入 swarm 走同一條規則。
- **重渲染一致性**（這是 suffix 放 def 而非 Option 的理由）：`ReloadSkills`、MCP
  目錄變動、`SwitchProfile` 的重渲染都經 `resolveMainProfileWithExtra` 從 registry
  重讀 def → suffix / OmitDate / 工具剝離**天然保留**，不需要 agent 多記狀態。
- skills 章節的 authoring 指引：`LongRunning` 時沿用 RP-10-3 的 omitAuthoring
  （長壽 swarm 成員不教自行寫 SKILL.md）。內建路徑經 `PromptContext` 新 bool
  （`OmitSkillAuthoring`，由 `def.LongRunning` 餵入）傳遞；磁碟路徑已有此 gate。

### 7.1 工具規則：全套帶入，僅剝離 solo 排程家族

persona 成員拿到人格的完整 kit（fs/shell/web/repl/lsp/subagent spawn/todo/skill/
config/MCP meta…）+ 角色對應 swarm 工具。唯一例外（以 `def.LongRunning` 為鍵，
在 `mainProfileForDef` 與 `mainProfileFromDiskAgent` 統一執行）：

> 從 active 與 deferred 兩列剝除：`alarm_create` / `alarm_list` / `alarm_cancel`、
> `cron_create` / `cron_list` / `cron_delete`、`schedule_wakeup`。

理由：swarm 的對應物是 `alarm_set`/`alarm_clear`（space 級、bus 投遞、roster 可見、
durable 重啟重掛）與 leader 的 `schedule_set`；solo 排程器與之並存會產生 roster
看不見的喚醒源。這條規則把既有手寫慣例（dir 成員 active.yml 註解裡的「不要列
solo alarm」）收進框架——對 dir 成員同樣生效（其 tools.yml 若誤列也會被剝）。

## 8. Skills 與記憶語義

**Skills**（優先序低→高）：`bundled` < `EVVA_HOME skills` < `workdir skills` <
`space 共享 agents/skills/` < `成員私有 agents/{main,sub}/<name>/skills/`。

- `pkg/skill.LoadRegistry(homeDir, workdirDir)` 改為 variadic
  `LoadRegistry(dirs ...string)`（後者勝；**源碼相容**——既有兩參呼叫照編譯，
  在 `docs/sdk-stability.md` 記一筆）。
- space 新增 `memberSkillRegistry(role, name) *skill.Registry` helper：persona 成員
  載上述五層（disk 四層 + `bundled.Register` 補洞）；dir 成員維持現行
  `(shared, member)` 兩層。**`constructMember` 與 `Supervisor.ReloadMemberSkills`
  都改走這個 helper**——建構與 reload 永遠同一組來源（RP-26 lockstep 慣例），
  `skill_publish` 的全員 reload 對 persona 成員同樣生效。

**記憶**：persona 成員 = swarm 原生成員記憶（RP-25 一致治理）：
`agents/{main,sub}/<name>/memory/`、wake 注入 MEMORY.md 索引、寫入 carve-out、
隊友可讀、web Memory 分頁可見；`EnableAutoMemory`/`EnableMemoryRecall` 維持
swarm 的強制關閉（solo 的 appHome 全域記憶**不**注入——成員的長期記憶屬於該
swarm，不污染人格的個人記憶，反之亦然）。workdir `EVVA.md`（專案慣例簡報）照
solo 規則注入（persona 成員不 OmitMemory）——這是宿主專案對 persona 成員的
briefing 通道。

## 9. 錯誤處理一覽

| 情境 | 行為 |
| --- | --- |
| 成員項 `agent:`/`persona:` 皆空或皆填 | `LoadManifest` 拒絕整份 manifest |
| persona 名與其他成員重名 | `validate()` 拒絕（涵蓋兩形態） |
| `effort:` 非枚舉值 | `LoadManifest` 拒絕 |
| persona 不在 registry / 非 main-tier | space 組裝失敗（`NewSpace` 報錯點名成員；service register 把錯誤回給 operator） |
| persona 成員目錄不存在 | **不是錯誤**（這正是本功能；記憶目錄由 `constructMember` 自動建立） |
| 重啟重建（service Reconcile → BuildAll → NewSpace） | persona 成員與 dir 成員同路徑重建；runtime schedule/membership 規則不變 |

## 10. 相容性

- **solo 零影響**：space registry 是每 space 私有副本；`Main()`/`ResolveMainProfile`
  既有簽名與行為不變（新邏輯都 gate 在 def.LongRunning / def.PromptSuffix 非零值上，
  solo 的這兩個欄位恆為零值）。
- **SDK**：`pkg/agent.AgentDefinition` 增欄位（additive）；`pkg/skill.LoadRegistry`
  變 variadic（呼叫相容）。downstream 編譯測試（converged_downstream_test 等）照跑。
- dir 成員 prompt 拼法不變（協議仍在 body 內）；唯一行為差異是 §7.1 的剝離規則
  開始對 dir 成員的誤列 solo 排程工具生效——這是修正而非回歸。

## 11. 測試計畫

- `agentdef`：persona 成員解析（互斥鍵、唯一性、effort 驗證、model/effort/when_to_use
  載入）、`WriteManifest` round-trip（persona 鍵保留）、dir 成員 manifest 覆寫。
- `agentdef/loader`：BuildAll 合成 persona Loaded（不碰磁碟）、dir 成員覆寫迴圈。
- `internal/swarm`：registerDef persona 路徑（unknown persona / 非 main-tier 失敗；
  WhenToUse/Model 覆寫；suffix 含五章節；space-local 不污染外部 registry）、
  constructMember persona 建構（記憶目錄、roster when_to_use）、
  memberSkillRegistry 五層優先序、ReloadMemberSkills 對 persona 成員的來源一致性、
  一條 persona member 起 space 的整合測試（member_test.go 既有 stub LLM 模式）。
- `internal/agent`：mainProfileForDef 的 OmitDate / suffix / 工具剝離；
  ReloadSkills 重渲染後 suffix 仍在；AgentSpec/pkg DTO round-trip 帶 PromptSuffix；
  Main() solo 路徑 bit-stable（無回歸）。
- `pkg/skill`：variadic LoadRegistry 多目錄優先序 + 兩參相容。

## 12. 文件與版本

- `docs/roadmap/veronica/user-guide-{en,zh}.md`：新節「Persona members」（yaml 形態、
  能力語義、skills/記憶規則、剝離清單）。
- `docs/roadmap/veronica/refine-plan/RP-29-persona-members.md`：薄指標，指向本 spec。
- `CHANGELOG.md` `[Unreleased]` ### Added。
- `CLAUDE.md` wave→minor 表追加：`v1.7 | Persona members（RP-29，第六波開場）`。
- 一律 `feature/persona-members` → PR → `dev`；release cut 由 operator 指令觸發
  （不在本工作範圍）。

## 13. v1 範圍外

- web 表單熱加 persona 成員（manifest 路徑 + 重啟重建已覆蓋營運需求）。
- 同一人格多實例（replicas）。
- 遠端 persona endpoint（registry 遠端註冊是另一個 roadmap 項）。
- persona 成員的 appHome 全域記憶橋接。
