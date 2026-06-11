# EX-3 — Leader 單點的退化保護（operator 接管優先，deputy 後議）

> 狀態：**探索 / Exploration** ｜ 日期：2026-06-10 ｜ 規模：中 ｜ 依賴：RP-14（stall 訊號）

## 動機

Leader 是結構性單點：不可移除（`supervisor.go:205-207`）、task 狀態寫權唯一
（store 的 leader-only 守衛）、所有 webhook/排程匯流到它。**leader 卡死或反覆 abort
＝全隊停擺**——workers 還會醒，但產出無人驗收、無人派工。RP-14 讓「leader 卡了」會
報警；本探索回答「報警之後呢」。

## 假設

不需要（也不該先做）自動 deputy 選舉——單 operator 模型下，**「User 一鍵接管」**就能
覆蓋 95% 的事故：把 stall 告警直接接到一組恢復動作上，比讓另一個 LLM 接管決策權
安全得多。

## 最短驗證（spike）

Web Attention 的 leader-stall 條目掛三個按鈕（後端 API 全是現成的）：

1. **Cancel run**（= Suspend→Resume：取消當前 run、mail 重投、回 idle）；
2. **Direct message**（operator 以 `user` 身分發信給 leader——既有
   `POST /api/agents/{name}/message`）；
3. **HaltAll**（全隊急停，既有 `POST /api/halt`）。

加一條**唯讀降級**：leader 連續 N 次 run abort（RP-17 的 counter）→ 自動發 User 通知
＋ 暫停 leader 的 timer schedule（workers 的巡邏照跑，只停「決策者的自驅」）。

## 成功訊號

- 人工製造 leader 卡死（fake 長 run）後，operator 在 web 上 30 秒內恢復隊伍運作，
  全程不需要 CLI；
- 誤觸 Cancel 不丟工作（unclaim 重投語意已有測試保障）。

## 風險與邊界

- deputy（臨時提升某 worker 的 task 寫權）牽動 store 守衛與 teamprompt 角色敘事，
  **本探索明確不做**——等接管 UX 用過幾次、知道真實事故形態再議；
- 自動化要克制：除了「暫停 leader 自驅」外不做任何自動決策。

## 依賴／去向

依賴 RP-14 的 stall 事件與 RP-17 的 abort counter。spike 即近似可交付（多為 FE 接線），
順利的話直接併入 FE 的態勢感知迭代，不必獨立 RP。
