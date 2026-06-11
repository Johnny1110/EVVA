# RP-25 — 成員原生長期記憶（EX-1 的畢業票：工具組 + wake 注入 + web 檢視）

> 狀態：**草案 / Draft（待 Johnny 拍板；可與 EX-1 spike 並行——見 §5）** ｜ 階段：**第五波** ｜ 優先：**P1** ｜ 日期：2026-06-11
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
