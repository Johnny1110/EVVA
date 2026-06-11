# RP-25 — 成員原生長期記憶（EX-1 的畢業票：工具組 + wake 注入 + web 檢視）

> 狀態：**✅ 已實作（2026-06-11，feature/RP-25；Johnny 拍板跳過 spike 直接動工）** ｜ 階段：**第五波** ｜ 優先：**P1** ｜ 日期：2026-06-11
> 觸發：Sunday swarm 重整。`/api/memory` 模式的生產證據又厚了一層：記憶名冊擴到 **7 個 agent**（新增 trader），且長出了框架該注意的協作模式——「共識雙副本」（friday 權威版 + risk-monitor 對照版，不一致 = 事故）與「User 在 dashboard Memory 分頁讀全員記憶」的透明性價值。每個新 swarm 重新發明這套的成本已被證實。
> 關聯：**[EX-1](../explore/EX-1-member-native-memory.md)（parent——本文是它「spike 成功 → 立 RP」承諾的那張 RP）**、[RP-5](RP-5-member-prompt-env.md)（bit-stable 前綴——索引必須走 wake 注入）、`internal/memdir`（複用的 typed memory 體系）、[RP-19](RP-19-disk-persona-tool-grounding.md)（記憶工具的使用準則經由其 mechanics 區塊教學）
> 請求者：Sunday。實作後 Sunday 的 `/api/memory` 降級為「User 視窗」或直接退役。

---

## 1. Problem（observed —— EX-1 動機 + 重整後的增量證據）

EX-1 已論證：長期成員的「心智」第三層（外部 KV 記憶）是真需求、但 Sunday 專屬。重整再補三點：

1. **規模證據**：Sunday 現在 7 個 memory-bearing agent，每份 persona 都要教一遍「醒來先 GET、收工前整份 PUT、每條標日期、過期刪掉」——這段紀律文字在 8 份 prompt 裡重複，正是框架該注入的協議。
2. **整份覆寫的弱點浮現**：單一 markdown 文件 PUT 整份覆寫，一次寫壞 = 全失憶（無類型、無 index、無局部更新）。memdir 的 typed-files + index 天生強於這個模式。
3. **可讀性是功能**：User 在 dashboard 讀全員記憶 = 團隊心智對 operator 透明。這個價值該在 evva Web 原生存在，而不是每個宿主 app 自己蓋頁面。

## 2. Proposal（EX-1 的三件套，正式化）

1. **儲存**：per-member memdir at `<workdir>/agents/{main,sub}/<name>/memory/`（與 persona 同住、跟著 agents/ 一起進 git 或被 .gitignore——operator 自決），完整複用 `internal/memdir` 的 typed frontmatter + `MEMORY.md` index。
2. **工具面**：成員獲得路徑約束的記憶讀寫（薄包 fs 工具或複用 solo 記憶 carve-out：**只能寫自己的** memory dir；讀全員開放——對齊 Sunday「analyst 可讀 friday 的 watchlist」慣例）。
3. **注入面（cache 紀律，EX-1 假設 2）**：`MEMORY.md` 索引掛在**喚醒訊息**的 `<system-reminder>`（與 currenttime 同一條），**絕不**進靜態 prompt——前綴 bit-stable（RP-5）不被記憶變動打爆。
4. **協議面**：teamprotocol 注入一段記憶紀律（醒來看索引、需要才讀全文、收工前更新、過期修剪、每條標絕對日期）——把 Sunday 8 份 prompt 的重複段收編成框架文字。
5. **Web**：每成員一個 Memory 檢視 tab（唯讀；User 終審可刪檔）——FE-7 範疇，列驗收但可後送。

## 3. Why evva（not Sunday）

EX-1 寫得透徹：第三層記憶已被生產驗證，但實作困在宿主 app。memdir 體系現成、wake 注入管線現成（RP-7 的 scheduledWakePrompt）、治理模型清晰（寫己讀眾）——缺的只是按成員實例化的膠水。

## 4. Acceptance

- 成員 wake 看到自己的 MEMORY 索引（在 wake reminder，靜態 prompt 逐位元不變——以 prompt 快照測試斷言）。
- 成員寫他人 memory dir 被拒；寫自己的免審批（比照 solo auto-mem carve-out）。
- 重啟後記憶完好（純檔案，天然 durable）；`evva swarm add` 熱加入的成員自動有空 memory dir。
- teamprotocol 含記憶紀律段；無 memory 內容的成員 wake 不掛空索引（零噪音）。
- Sunday 遷移驗證：researcher 用原生記憶跑一週，「接續上次線索」品質不輸 `/api/memory` 組（EX-1 的成功訊號原樣沿用）。

## 5. Notes

- **與 EX-1 的關係**：spike（researcher 單成員、一週對照）仍值得先跑——本 RP 的 §2.3 注入位置與 §2.2 工具面就是 spike 要驗的兩個假設。spike 否證任一假設 → 回本 RP 修設計再動工；Johnny 也可基於 Sunday 既有證據直接拍板跳過 spike。
- **與 compaction 的銜接**（EX-1 標記的第二步，本 RP 仍不做）：full-compact 前把摘要寫回 memory——另立後續。
- Sunday 的 `/api/memory` 不必立即退役：dashboard Memory 分頁仍是 User 視窗；過渡期兩存並行，agent 端切原生後再評估。

---

## 6. 落地註記（2026-06-11）

§2 三件套全落地（spike 跳過——user 直接 `25 go`）。實作走「複用 solo carve-out」線，零新工具：

1. **儲存 + SDK 縫**：`agentdef.MemoryDir(workdir, role, name)` = `<agentDir>/memory/`，
   constructMember `MkdirAll`（首啟與 hot-add 同路徑 → AC#3 的空 dir 自動成立）。關鍵新縫是
   `pkg/agent.WithMemoryDir(dir)`（內部 `memDirOverride`）：把該 agent 的可寫記憶**改家**到
   member dir——carve-out 與 recall 都改打這裡，且該店的 MEMORY.md **強制不進靜態 prompt**
  （override 同時清空 snapshot.MemoryIndex）。member clone 另把 `EnableAutoMemory`/
   `EnableMemoryRecall` 強制關閉：solo 的全域記憶 prompt 段（指向 `<appHome>/memory`，對成員
   是錯的店）與 per-turn recall side-query（×N 成員 ×每 wake 的 LLM 成本，與「索引 + 按需
   read」協議重複）都不該出現在成員身上。EVVA.md 注入（inject_memory: true 成員）行為不變。
2. **工具面 = fs 工具 + 雙向圍欄（pkg/permission）**：寫自己 → 既有 isAutoMemWrite carve-out
   免審批（memDir 已改家，免費獲得）；寫隊友 → 新 `isSiblingMemWrite` deny，位置在 deny 規則
   旁、**bypass 短路之前**（Sunday trader 是 bypass，AC#2 的「被拒」必須穿透 bypass——RP-24
   重排後 deny 級就是這個位置）。**自我 gating**：只有 memDir 本身在 `<workdir>/agents/` 下的
   agent（= swarm 成員）才受 fence 管；solo evva 在 swarm workspace 裡幫 operator 改成員記憶
   走正常 ask，不誤傷。已知界限（誠實記錄）：fence 只管 write/edit 檔案工具，**bash 寫檔不在
   其內**（與 solo carve-out 同界——bash 由 classifier + mode 治理；fence 是 guardrail 不是
   sandbox），紀律文字 + 讀眾透明是補償控制。`agents/{main,sub}` 佈局常數燒進 pkg/permission
   與 isPlanFileWrite 燒 `.evva/plans` 同類（pkg 不能 import internal/swarm）。
3. **注入面**：`sp.memoryWakeReminder(name)` 每次 wake 現讀 MEMORY.md（16KiB 上限），空 =
   ""（AC#4 零噪音）；`scheduledWakePrompt` / `composeMailPrompt` 加第三參數，索引**掛在
   currenttime 同一條 `<system-reminder>` 內**（ticket §2.3 原樣）。外部事件/alarm 都走 bus
   mail → composeMailPrompt 覆蓋。已知小縫：operator 從 Web 直接 chat 的 run 不經兩個 wake
   builder，**不帶索引**（該 run 本來也不帶 currenttime）——成員下個 timer/mail wake 補上；
   要不要給 web-chat 路徑同樣的 reminder 另議。bit-stable 驗收：space_test 用擷取式 stub
   provider 抓每個成員建構時的 system prompt，寫入記憶檔後重建 space 逐位元比對 + 斷言內容
   零洩漏（`TestMemberPromptBitStableAcrossMemoryChange`）。
4. **協議面**：`memoryProtocol(name, role)` 注入 teamprotocol（`## Your long-term memory`），
   收編 Sunday 8 份 prompt 的重複紀律段：一事一檔 frontmatter、絕對日期、收工前持久化、修剪、
   讀眾寫己、「不該存什麼」。**Gate 在 write|edit 工具上**——沒有寫檔工具的成員教了也做不到，
   純噪音；唯讀成員仍可讀隊友記憶（read 工具本來就 safelist）。
5. **Web**：唯讀 `GET /api/agents/{name}/memory?space=` 已上（`Backend.MemberMemory` →
   `sp.MemberMemoryFiles`，MEMORY.md 排首、單檔 64KiB cap）；**FE Memory 分頁與 User 刪檔
   留 FE 批次**（ticket 自註可後送）。
6. **AC#5（Sunday 遷移對照）**在 user 的 Mac 上跑，不在本票範圍；evva 側已備妥。
   共識雙副本（friday 權威 + risk-monitor 對照）這類模式現在直接用「讀眾」表達。
