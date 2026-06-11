# EX-4 — Replay／eval harness（重放事件流，迴歸評測 prompt 與模型版本）

> 狀態：**探索 / Exploration** ｜ 日期：2026-06-10 ｜ 規模：大 ｜ 依賴：**RP-17**（event log）

## 動機

Trading team 這類 swarm 的行為對 prompt 措辭與模型版本極度敏感（改一句巡檢提示，
friday 的下單行為可能整個變樣），但今天**沒有任何迴歸手段**——改了 prompt 只能上線觀察。
有了 RP-17 的 append-only event log，「把昨天重放給新版本」就從空想變成工程問題。

## 假設

1. swarm 的對外輸入面是有限的：external webhook（`SendExternal`，自帶 idempotency
   seam，`bus/bus.go:103`）、scheduled wakes（`alarm.Config.Now` 已有注入時鐘的先例）、
   operator messages。從 event log 抽出這三類即可重建「一天的輸入流」。
2. 在 staging space（同 manifest、staging workdir、**工具側效果隔離**）重放輸入流，
   比對兩個 prompt 版本的輸出面（messages、task 流轉、對外 HTTP 呼叫記錄），足以回答
   「新版本行為變好還是變壞」。

## 最短驗證（spike）

1. 寫一個 `evva swarm replay <ref> --from events/2026-06-09.jsonl --speed 60x` 雛形：
   只重放 webhook events（最單純的一類），按原時間間隔壓縮速度投遞。
2. 隔離面先用最粗的辦法：staging space 的成員 permissions 全鎖 read-only ＋ Sunday
   指到 testnet（Phase 2 的環境本來就是 testnet，天然安全）。
3. 比對工具：兩份 run 的 messages/tasks dump diff ＋ 人眼。

## 成功訊號

- 同一事件流重放兩次（同 prompt）行為大體一致（LLM 非確定性可接受，結構性決策一致）；
- 重放到改過 prompt 的版本，能在 diff 裡明確看到行為差異點——這就是 eval 的最小可用形態。

## 風險與邊界

- 完全確定性重放是非目標（LLM 本身非確定）；目標是**結構性迴歸**（該醒的醒了、該報
  的報了、不該下的單沒下）；
- 時鐘注入要全鏈路（cron Next、alarm、currenttime stamp）才能重現「當時幾點」——
  工程量主體，spike 先不做（用壓縮速度近似）；
- 外部世界（Binance 行情）不可重放——Sunday 側需要 record/replay 配合，屬 Sunday repo
  的對應探索，本檔只管 swarm 側。

## 依賴／去向

硬依賴 RP-17。spike 成功 → 立 RP 拆：輸入抽取器、時鐘注入、隔離執行、diff 報告四件。
