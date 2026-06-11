# RP-13 — 成員用量儀表＋每日預算熔斷（usage metering & budget breaker）

> 狀態：**✅ 已實作（2026-06-10，feature/RP-13-usage-metering）** ｜ 日期：2026-06-10 ｜ 波次：**第四波（運營硬化）** ｜ 優先：**P0**
> 觸發：2026-06-10 健康檢查——Sunday trading swarm 24/7 運轉（watchdog `*/2 * * * *` 一天
> 720 次喚醒、7 個成員混 Anthropic/DeepSeek），但 **token 燒在哪、燒多快，operator 完全看不到**。
> 上層：[`../health-check-2026-06-10.md`](../health-check-2026-06-10.md)

---

## 1. TL;DR

每個成員的 agent 層**早就有完整用量統計**，swarm 層卻一條都沒接出來（grep `Usage` 在
`internal/swarm` 零命中）。本 RP 分三層補齊：

1. **讀取面（零新管線）**：`Roster` 持有的就是 `ui.Controller`，介面本身已含
   `Usage() llm.Usage` 與 `LastTurnInputTokens()` —— `Snapshot()` 時順手取即可。
2. **顯示面**：`list_members`（leader 自省）＋ webapi `MemberInfo`（web roster pill）。
3. **治理面**：space 級 `daily_budget_tokens`（可 per-member override）；超額 → 自動
   `Freeze` ＋ 給 leader 與 User 各發一則通知 message。**這是 24/7 自治集群的保險絲。**

## 2. 現況盤點（file:line 證據）

| # | 事實 | 位置 | 意義 |
| --- | --- | --- | --- |
| S1 | `ui.Controller` 介面已含 `Usage()` / `LastTurnInputTokens()` | `pkg/ui/ui.go:96-103` | ✅ 讀取面 seam 現成 |
| S2 | session 累計用量 + 每輪 input tokens | `internal/agent/agent.go:1062,1066` | ✅ 資料源已在 |
| S3 | `Roster` 存每成員的 `ui.Controller` | `roster.go:121,173` | ✅ Snapshot 可順手取 |
| S4 | `MemberView` 無 usage 欄 | `roster.go:76-93` | ❌ 補欄位 |
| S5 | `list_members` 不顯示用量 | `tools/messaging.go:88-124` | ❌ 補顯示 |
| S6 | webapi `MemberInfo` 無 usage | `service/service.go:830`（`Roster()`） | ❌ 補欄位＋web pill |
| S7 | `Freeze` 已是一級操作（凍結不再排程、mailbox 保留） | `supervisor.go:232-239` | ✅ 熔斷動作現成 |
| S8 | manifest `settings` 已有擴充點（`permission_mode`、`max_iterations`） | `agentdef/manifest.go:55-60` | ✅ 預算欄位放這 |
| S9 | `runtime.json` 已持久化 membership/schedules | `resume.go:26-59` | 當日累計可比照持久化 |

## 3. 設計方向

### 3.1 讀取面：Snapshot 順手取

`Roster.Snapshot()` 在組 `MemberView` 時加讀 `ctl.Usage()` / `ctl.LastTurnInputTokens()`：

```go
type MemberView struct {
    // ...既有欄位...
    Usage         llm.Usage // cumulative session tokens (in/out/cache)
    LastTurnInput int       // context pressure of the most recent turn
}
```

注意：`Usage()` 走 session 內部讀，需確認無鎖衝突（agent loop goroutine vs Snapshot
caller）——`session` 既有併發契約若不足，這裡用 controller 既有的安全出口，不繞私路。

### 3.2 顯示面

- `list_members` 每行尾加 `· 1.2M tok (last turn 45k)` 級別的緊湊摘要。
- webapi `MemberInfo` 加欄位；FE roster pill 顯示（FE-5 態勢感知的自然延伸，攔截
  「context pressure 接近 compaction」也可一眼看出）。

### 3.3 治理面：每日預算熔斷

- `evva-swarm.yml` `settings.daily_budget_tokens`（0/缺省＝不限）；member 級
  `budget_tokens` override（profile.yml 或 manifest member 欄）。
- 計量單位：**當日累計 output+input tokens**（local 時區的日界線——時區語意沿
  v1.4.5-beta.2 的 `pkg/common` 約定）。
- 檢查點：`runOnce` 結束處（`scheduler.go:194-209` 附近）——run 完成才結算，不打斷
  進行中的 run。
- 超額動作：`Freeze(member)` ＋ Bus 各發一則給 leader 與 `user` 收件匣的說明
  message（含當日數字與解凍方式）；事件流發 `KindBudgetTrip`（web Attention 可接）。
- 當日 counter 持久化（`runtime.json` 加 `usage_daily` 或 vero.db 小表），重啟不歸零；
  跨日自動重置並自動 `Unfreeze`（可設 `stay_frozen: true` 改成人工解凍）。

## 4. 驗收（DoD）

1. `list_members` 與 web roster 都看得到每成員 cumulative tokens 與 last-turn input。
2. 設 `daily_budget_tokens` 後，超額成員在下一個 run 邊界被凍結，leader 與 User 都收到
   通知，web Attention 出現對應條目。
3. 重啟 service 後當日累計不歸零；跨日重置；解凍語意符合設定。
4. 不設預算時行為與現狀完全一致（純顯示）。
5. `-race` 綠燈（Snapshot 讀 Usage 的併發路徑有測試）。

## 5. 非目標

- 精確金額計價（model 單價表、幣別）——先 tokens，價格表是後續小 PR。
- 歷史報表/圖表——先當日與 cumulative；歷史落 RP-17 的 event log 再聚合。

---

## 6. 實作記錄與偏離（2026-06-10）

落點：`internal/swarm/usage.go`（meter + 熔斷狀態）、`scheduler.go`（run 邊界計量
`meterRun`/`tripBudget`/`sweepBudgetDay`）、`roster.go`（usage 快照欄）、`resume.go`
（runtime.json 持久化）、`agentdef/manifest.go`（設定欄）、`tools/messaging.go` 與
`webapi`/`service`（顯示面）。測試：`usage_test.go` 六條 + manifest round-trip +
fmtTokens；`-race` 全綠。

與原案的偏離：

1. **讀取面比原案更嚴**：§3.1 原想在 `Snapshot()` 順手呼叫 `ctl.Usage()`——實查
   `internal/session` 無鎖，任意 goroutine 讀會與 run 中的寫競態。改為 supervisor 在
   **run 邊界**（成員自己的 loop goroutine）讀前後差值寫進 roster 快照，顯示面只讀快照；
   `startMemberLoop` 啟動時種子一次（resume 後的累計立即可見）。web 既有的
   `ContextTokens` live-read 先例維持原樣，不擴大。
2. **熔斷的翻日釋放改為「標記自帶觸發日」**：實作中發現原設計的「sweep 觀察 day 變化」
   有個會偷走釋放邊緣的洞——任何成員在午夜後、sweep 前跑完一輪，`ensureMeter` 就把
   計數日推進，sweep 永遠看不到變化，凍結成員（永不 run）就永遠卡住。現在
   `frozen: map[name]→tripDay`，sweep 判「標記日 ≠ 今天」，邊緣不可偷。
3. **`KindBudgetTrip` 事件暫緩**：凍結狀態 web roster 本就可見、通知信會進 Timeline，
   專屬事件種類等 FE 真要接 Attention 再加（pkg/event 是 SDK 面，不為死線路擴）。
   FE roster pill 同樣留給 FE track（API 欄位已備好）。
4. per-member override 放 **manifest**（團隊 cadence 與預算同檔），不放 profile.yml；
   通知 sender 用 `"system"`。手動解凍 = 操作員覆寫：清標記，仍超標則下個 run 後
   **恰好再觸發一次**（測試覆蓋）。
