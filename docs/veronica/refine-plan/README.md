# Veronica — Refine Plans（smoke-test 後的修整計畫）

> 狀態：**草案 / Draft（待 Johnny 拍板）** ｜ 日期：2026-06-05
> 觸發：`evva swarm` 第一次 smoke test 暴露的三類問題。
> 上層設計：[`../veronica-design-v1.md`](../veronica-design-v1.md) ｜ 路線圖：[`../roadmap.md`](../roadmap.md)
> 相關方向：[`../direction-flat-comms.md`](../direction-flat-comms.md)

---

## 背景

Phase 1（SPRD-1-1 ~ 1-13）已讓 swarm 主幹跑起來，但第一次 smoke test 發現三個會
**直接讓團隊卡死**的問題。三份計畫各對應一個問題，並做了完整 source-code review
（含 file:line 證據）後提出修整方向。**目前是開發階段，允許大動作重構** —— 凡發現
原設計有結構性缺陷處，計畫直接提出重新設計，而非打補丁。

| # | 計畫 | 對應問題 | 嚴重度 | 核心結論 |
| --- | --- | --- | --- | --- |
| [RP-1](RP-1-messaging-reliability.md) | 訊息投遞可靠性 | A↔B 訊息漏收 / 卡 unread | 🔴 高 | `drainStaleHints` 過度清空 mailbox hint → **lost wakeup**；喚醒只靠 chan hint、未對 DB 對帳。改為 **DB 權威的 level-triggered drain**。 |
| [RP-2](RP-2-permission-broker-routing.md) | Permission broker 卡死 | 審批框出不來、agent 卡 busy | 🔴 高（**deterministic**） | 前端用 `AgentID`(UUID) 回傳審批，後端卻用**成員名**查 controller → 路由必失敗、回應被丟掉。**每一次 web 審批都會 hang。** 另含單槽審批互蓋、無 reconnect 重放。 |
| [RP-3](RP-3-agent-run-phase-states.md) | Agent 狀態過粗 | 只有 busy，卡住看不出卡在哪 | 🟡 中 | Roster 只有 `idle/busy/suspended`，且由 supervisor 手動設定。改為**從 event stream 推導**的細狀態（RUNNING / EXECUTING / WAITING_APPROVAL / …），對齊 evva TUI。 |
| [RP-4](RP-4-web-ui-ux.md) | Web UI/UX 檢討 | 介面像「聊天 App」、缺態勢感知 | 🟡 中（設計方向） | 以資深設計師視角做啟發式評估；重定義為「**swarm operations console**」（監看＋介入），優先補 **Attention Bar / 審批 tray / Team Timeline**。**純設計方向文件、未實作。** |

> RP-1~3 是 smoke-test 暴露的**功能 bug 修整**（已實作，見下方狀態）；**RP-4 是另一條
> UI/UX 設計方向 track**（檢討＋方向，尚未動工）。

## 三者的關係（RP-1~3 功能修整）

```
RP-2（修好審批路由 + 並發 + 重放）  ←──診斷靠──┐
                                             │
RP-3（WAITING_APPROVAL 等細狀態）  ──讓 RP-2 的卡死「看得見」
                                             │
RP-1（訊息不再漏 / 不再卡 unread）  ←── 共用「DB 為唯一真相」哲學
```

- **RP-2 是最該先修的** —— 它是 deterministic bug，不是 race，目前任何走 Web 的審批
  都必然卡死。
- **RP-3 是 RP-2 的觀測面** —— 有了 `WAITING_APPROVAL` 細狀態，「卡在審批」這件事在
  UI 上一眼可見，而不是泛泛的 busy。兩者一起做收益最大。
- **RP-1 獨立但同源** —— 訊息可靠性問題與審批無關，但解法同樣是「不要相信記憶體 hint，
  以 SQLite 落地的真相為準」。

## 建議落地順序

1. **RP-2 §1（路由修復）** —— 最小改動、解掉 deterministic hang，先讓 demo 能跑。
2. **RP-1（訊息可靠性重設計）** —— 解掉「漏收 / 卡 unread」，團隊協作才可信。
3. **RP-3（細狀態）** + **RP-2 §2–§5（並發、重放、HOL）** —— 觀測面與韌性，一起收尾。

> 每份計畫的 Acceptance / DoD 皆可獨立驗收；三者無強制先後依賴（除上述建議順序）。

---

## 實作狀態（2026-06-05 落地）

三份計畫**皆已實作於 `feature/veronica`**（build + vet + `-race`（swarm/cmd）測試綠燈、
depcheck clean、web `npm test` + build 完成）。

| 區塊 | 狀態 | 重點落點 |
| --- | --- | --- |
| RP-2 §3.1 路由 | ✅ | `Roster.ControllerRef`（名稱或 AgentID）、`dispatchInbound` 不再吞錯 + `command_error` WS frame；**走 UUID 路徑的回歸測試** |
| RP-1 訊息可靠性 | ✅ | 新 migration `0002_message_claim.sql`（`claimed_at` 三態）+ `ClaimUnread/ClaimOne/SettleClaimed/UnclaimFor`；scheduler 改 **level-triggered** `serve`+`runOnce`、**刪 `drainStaleHints`**、safety `rescanTick`；drain B 改 claim；重啟 `UnclaimFor`；role-addressing |
| RP-3 細狀態 | ✅ | `RunPhase` + `phaseDeriver`（移植 TUI `status.State`）+ sink 推導寫回 roster；`MemberView.Phase/Tool` + `DisplayPhase`；`list_members` / webapi / web roster pill |
| RP-2 §3.2 並發審批 | ✅ | 前端審批/問題改**佇列**、不互蓋 + 「N pending」徽章 |
| RP-2 §3.3 重放 | ✅ | service `gateTracker` + `GET /api/swarm/:id/pending` + 前端 WS reconnect 時 hydrate |
| RP-2 §3.5 防凍結 | ✅ | hub WS `wsWriteTimeout` + 壞連線淘汰 |
| RP-1 §3.6 console 訊息可見化 | ⏸ **deferred** | 純 UI nicety（inter-agent 訊息目前已可在右欄 AgentTranscript mailbox 看到）；留作後續 |

> 設計決策偏離：RP-3 原案建議「移除 supervisor 手動 setRun、全改 event 推導」。實作改為
> **保留 coarse `run`（idle/busy/suspended，supervisor 權威、event-less 測試 controller 也能用）
> ＋ 疊加 event 推導的 fine `phase`**，前端/`DisplayPhase` 合成顯示（suspended 優先 → phase → coarse）。
> 理由：測試以無 event 的 fake controller 驅動，純 event 推導會讓它們失去狀態；coarse + fine 兩層
> 同時相容測試與真實 agent，且 web-`Run` 路徑也因 event 推導而自動正確。
