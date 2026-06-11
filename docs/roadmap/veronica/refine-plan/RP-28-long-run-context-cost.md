# RP-28 — 長壽成員的 context 成本（Part A：per-run token 計量；Part B：fresh-context wake 設計方向）

> 狀態：**草案 / Draft（待 Johnny 拍板；Part B 為設計方向、依 Part A 數據開工）** ｜ 階段：**第五波** ｜ 優先：**P2** ｜ 日期：2026-06-11
> 觸發：Sunday swarm 重整。團隊跑 7×24、成員一天醒 8～480 次（researcher 3 次 ↔ watchdog 每 3 分鐘），**對話史只增不減直到 compaction**——但絕大多數喚醒是事務性的（巡檢、看板掃一眼、stand down），背著全史醒來純屬成本。今天連「每次喚醒花多少」都看不到，優化無從談起。
> 關聯：[RP-13](RP-13-member-usage-metering.md)（成員級累計/當日計量——本文補 **per-run** 粒度）、[RP-17](RP-17-durable-event-log.md)（run 生命週期事件——本文往裡面掛 token 欄位）、[RP-5](RP-5-member-prompt-env.md)（prompt 前綴 cache 紀律——本文的數據可量化它的實效）、[RP-25](RP-25-member-native-memory.md)（Part B 的前置：記憶是 fresh-context 的種子）、[EX-4](../explore/EX-4-replay-eval-harness.md)（Part B 改動的行為回歸靠它把關）
> 請求者：Sunday。**無 Sunday-specific code。**

---

## 1. Problem（observed）

1. **計量粒度斷層**：RP-13 給了累計與當日（`tok in 1.2M out 345k, today 89k/500k`），但回答不了「watchdog 每次喚醒多少 token、趨勢有沒有隨對話變長而爬升」——**per-run 數字不存在**，cache 命中與否更是黑箱。
2. **結構性成本假說（待數據證實）**：長壽成員的每次喚醒都攜帶整段對話史。靜態前綴有 RP-5 保 cache，但**對話段**隨運營單調增長；對「醒來→兩個 GET→stand down」的事務性喚醒，這是純開銷。watchdog 一天 480 醒 × 漸長的史 = 帳單裡最大的可疑項。
3. 沒有數據前任何「改 context 策略」都是盲動——所以本 RP 把**計量（Part A）作為可驗收主體**，把**策略（Part B）留作設計方向**。

## 2. Proposal

**Part A — per-run token 計量（concrete，本 RP 的驗收主體）**：

1. run 結束事件（RP-17 event log 既有 `run` 生命週期）加欄位：`tokensIn / tokensOut / cacheRead / cacheWrite`（provider 回報有什麼掛什麼，缺的留 null——不偽造）。
2. `/metrics` per-member 加 run-token 直方圖（桶：lt1k / lt10k / lt50k / gte50k，比照 runSeconds 既有做法）。
3. Web member 檢視加一條 per-run token 火花線（FE-5 範疇，可後送）。
4. 一句 grep 能回答：「watchdog 本週的 per-run tokensIn 趨勢」——`jq` event log 即得。

**Part B — fresh-context wake（design direction，不在本 RP 驗收）**：

- 構想：成員或 schedule 可標 `wake_context: fresh`——該喚醒不帶對話史，以「靜態 prompt + 記憶索引（RP-25）+ 本次 wake reminder + mailbox 未讀」冷啟。事務性巡檢（watchdog、保護腿檢查）是天然候選；連續性工作（leader、研究線索）維持 full。
- 風險直球：失去「上次喚醒的短期脈絡」可能讓行為變笨——**必須**等 Part A 數據劃出受益面、且有 EX-4 replay 做行為回歸，才值得動工。屆時另立 RP，本文只佔位設計方向。

## 3. Why evva（not Sunday）

token 計量在 LLM client / run 邊界，context 組裝在 agent runtime——兩者都不是宿主 app 可觸及的層。Sunday 只能看 RP-13 的日累計猜成本結構。

## 4. Acceptance（僅 Part A）

- 每個 run 結束事件帶 token 欄位；event log 一天的檔案可重建任一成員的 per-run 序列。
- `/metrics` 出現 per-member run-token 直方圖；計數與 RP-13 的累計對得上（同源不二記）。
- provider 不回報 cache 欄位時優雅留 null，不阻塞事件。
- 計量失敗不影響 run 本身（與 event log「永不回壓」同紀律）。

## 5. Notes

- Part A 落地後先回答三個問題再談 Part B：① watchdog 類成員的 per-run 成本是否隨史增長？② compaction 觸發頻率與成本峰值的關係？③ RP-5 的前綴 cache 實際命中率？——答案可能是「compaction 已經夠用」，那 Part B 就永遠不立案，這是好結局。
- 與 retention（RP-16）無涉：那清的是 ledger，不是對話史。
