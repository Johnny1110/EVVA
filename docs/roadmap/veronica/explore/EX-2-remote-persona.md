# EX-2 — Remote persona（RemoteController：遠端 runtime 入隊）

> 狀態：**探索 / Exploration** ｜ 日期：2026-06-10 ｜ 規模：大 ｜ 依賴：無（安全面建議先過 RP-15）

## 動機

專案願景（CLAUDE.md）：persona 是一級公民，外部專案可以註冊自己的 persona——
「a future `nono` web service registers as a remote agent endpoint」。今天的 swarm 是
process model A（一個 process host 一切）；但健康檢查確認了關鍵 seam 一直守著：
**`Roster` 對成員只認 `ui.Controller` 介面**（`roster.go:121,173`），bus/store/supervisor
從不觸碰具體 agent 型別。

## 假設

做一個 `RemoteController`（實作 `ui.Controller`，Run/Stop/Usage… 轉成對遠端 runtime 的
HTTP/WS 呼叫），遠端 persona 即可入隊——**bus、store、scheduler、task 狀態機一行不改**：
remote member 的 mailbox 仍在本 space，喚醒語意不變，只有「執行」發生在別處。

## 最短驗證（spike）

1. 定義最小 wire：`POST /run {prompt}` → SSE/WS 回傳 event 流 ＋ 最終 output；
   `POST /stop`；`GET /usage`。
2. 用第二個 evva process 起一個 echo persona 服務（loopback），手寫 RemoteController
   接進 manifest（`member.kind: remote, endpoint: …`），讓 leader send_message 它、
   它回信、task assign/verify 走完一輪。

## 成功訊號

- list_members 看得到 remote 成員的 phase（event 流接上 spaceSink 推導即得 RP-3 細狀態）；
- 斷線語意清楚：remote 掛了 → run abort → mail unclaim 重投（複用既有非乾淨退出路徑），
  不需要新恢復機制。

## 風險與邊界

- **事件回傳協議**是工程量主體（streaming chunk → event.Event 序列化）；
- **權限門遠端化**先不做：spike 限 read-only/bypass persona，approval 流跨進程是後續；
- 安全：endpoint 認證沿 RP-15 的 token 思路（per-remote shared secret）；
- 明確非目標：跨 space bus、分散式 store——process model A 的店面不動，動的只有執行面。

## 依賴／去向

spike 成功 → 立 RP 拆三件：wire 協議定稿、RemoteController 正式化（含 reconnect）、
manifest/`add-member` 支援 remote kind。這條線同時就是 `evva → nono` 委派願景的落地路徑。
