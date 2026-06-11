# EX-5 — 喚醒 jitter（拆掉整點齊醒的 thundering herd）

> 狀態：**探索 / Exploration** ｜ 日期：2026-06-10 ｜ 規模：**小** ｜ 依賴：無

## 動機

Sunday 的班表有三個成員在整點同刻醒（`0 * * * *` ×2、`0 0,8,16 * * *`，reviewer 又疊在
00:00）——同一秒一起打 LLM API 與 Sunday engine。成員數放大後就是自找的 429/延遲尖峰。
solo 端的 `cron_create` 描述早就寫了「避開 :00/:30、挑 7 或 57 分散負載」，swarm 的
schedule 卻沒有對等機制。

## 假設

per-member 的固定偏移（deterministic jitter）就夠：同一成員每次偏移一致（行為可預期、
日誌可對齊），不同成員彼此錯開。不需要隨機抖動。

## 最短驗證（spike）

`Schedule` 加可選 `jitter`（如 `90s`）：`Next()` 結果 ＋ `hash(memberName) % jitter`。
一行 config、十幾行實作＋測試。在 Sunday 開啟後觀察一天：整點的 API 延遲/429 與
Sunday 端瞬時 QPS 是否平滑。

## 成功訊號

- fireDue 在整點不再同刻 poke 多個成員；
- Sunday engine 整點的請求尖峰攤平；
- 日誌裡每個成員的偏移固定（可預期性保住）。

## 風險與邊界

- reviewer 的「當日復盤」這類**語意上就該在 00:00** 的班次要能豁免（`jitter: 0`）；
- 文件要講清楚：jitter 改變的是觸發時刻，不是 cron 語意（list_members 顯示班表時
  順手標 `(+37s)`）。

## 依賴／去向

無依賴、隨時可做。spike 即近似完成形態——驗證後直接併入 RP-18 級的小 PR，不必獨立 RP。
