# EX-1 — 成員原生長期記憶（把 Sunday 的 /api/memory 模式泛化）

> 狀態：**探索 / Exploration** ｜ 日期：2026-06-10 ｜ 規模：中 ｜ 依賴：無（建議排在 RP-13/14 後）

## 動機

長期運行成員的「心智」今天靠三層：session resume（重啟不失憶）、auto-compact（上下文
不爆）、以及——**Sunday 團隊自己發明的** `/api/memory`（compaction 會磨掉長期結論，所以
agents 把 watchlist/風控共識/研究線索整份寫進外部 KV，醒來先 GET）。第三層證明了真實需求，
但它是 Sunday 專屬的；換一個 swarm 場景就要重新發明一次。

evva 本體已有現成的 typed memory 體系：`internal/memdir`（user/feedback/project/reference
四型、frontmatter、index）。**假設：把 memdir 按成員實例化，就是 swarm 原生的長期記憶。**

## 假設

1. per-member memdir（`<workdir>/agents/<role>/<name>/memory/`）＋ 喚醒時注入 MEMORY 索引，
   能取代 Sunday 式外部 KV 的大部分用途，且對所有 swarm 通用。
2. 索引放**喚醒訊息**而非靜態提示詞（沿 RP-5 哲學：靜態前綴 bit-stable 保 cache，動態
   內容走 wake）——記憶變動不會打爆 prompt cache。

## 最短驗證（spike）

給一個成員掛 memdir 讀寫工具（read/write/list 三個，複用 solo 的記憶工具或薄包 fs 工具
＋路徑約束），`scheduledWakePrompt` / `composeMailPrompt` 尾掛 MEMORY.md 索引（一行一條，
與 currenttime 同一個 system-reminder）。在 Sunday 給 researcher 跑一週，對照組維持
`/api/memory`。

## 成功訊號

- researcher 的「接續上次線索」品質不輸 `/api/memory` 組（journal 可比對）；
- 記憶檔保持精簡（agent 自己會修剪——這是 memdir 約定的一部分）；
- prompt cache 命中率無明顯回落（wake 注入不在前綴）。

## 風險與邊界

- 記憶膨脹 → 索引行數上限＋「過期標記/刪除」紀律寫進 teamprompt；
- 與 compaction 的關係值得順手探：full-compact 的摘要產物 hook 一份寫回 memory
  （「壓縮前先存檔」）——但這是第二步，spike 不做；
- **治理**：成員只能寫自己的 memory；leader/User 可讀全員（debug 視角，web 後續）。

## 依賴／去向

無硬依賴。spike 成功 → 立 RP（工具組、wake 注入、web 檢視三件套）。
