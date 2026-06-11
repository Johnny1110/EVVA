# RP-27 — CLI 成員訊息（`evva swarm send <ref> <member> <text>`）

> 狀態：**草案 / Draft（待 Johnny 拍板）** ｜ 階段：**第五波** ｜ 優先：**P2（小件）** ｜ 日期：2026-06-11
> 觸發：Sunday swarm 重整——8 份 persona 全部重寫後，逐一驗證「analyst-flow 收到指派會不會正確回報」這類行為，目前只有兩條路：開 Web UI 手點、或繞道 webhook（只能喚醒 `to:` 指定的單一收件人、且 sender 是 `webhook` 語義不對）。**prompt 迭代的驗證迴路缺一個可腳本化的入口。**
> 關聯：`internal/swarm/service/service.go:1099`（`SendUserMessage`——後端原語已存在，Web 專用）、[RP-9](RP-9-external-event-webhook.md)（外部事件入口；本文是 operator 訊息入口，sender 語義不同）、`cmd/evva/swarm.go`（現有子命令：run/stop/rm/reset/add/vacuum——無 send）
> 請求者：Sunday。**無 Sunday-specific code。**

---

## 1. Problem（observed）

「以 user 身分對任一成員說話」這個能力後端早就有（`SendUserMessage`：sender=`user`、走 bus、drain A/B 喚醒語義齊全），但只接在 Web 控制台上。後果：

1. **prompt 迭代不可腳本化**：改完 persona 想驗「收到 X 會不會做 Y」，得人肉開瀏覽器；八個成員一輪迴歸 = 八次手點。
2. **headless 環境無入口**：CI / ssh-only 機器上的 swarm 完全沒有 operator 訊息通道（webhook 不等價——sender 是 `webhook`、會被 teamprotocol 教成「外部事件」而非「User 指示」）。
3. 與 CLI 既有能力不對稱：能 `add` 成員、能 `vacuum` 帳本，卻不能對成員說一句話。

## 2. Proposal

1. 新子命令：

   ```sh
   evva swarm send <ref> <member> "訊息文字"
   #  → 經 service HTTP API 呼叫 SendUserMessage（sender="user"）
   #  → 印出 message id；idle 成員隨即喚醒、busy 成員折進當前 run（既有 drain A/B）
   ```

2. **webapi 補一個薄端點**（若 Web 現走 WS command 通道）：`POST /api/swarm/{ref}/members/{member}/message`，token 鑑權與其他 webapi 一致（RP-15 的 minted token；CLI 讀 `~/.evva/service/token`，與 `swarm add` 同模式）。
3. stdin 支援（`-` = 從 stdin 讀 body）方便長訊息與腳本管道。
4. 明確**不做**的：等待回覆（fire-and-forget，與 Web 同語義）、廣播旗標（要廣播就對 leader 說讓他轉——保持 operator→member 是一對一原語）。

## 3. Why evva（not Sunday）

這是 swarm 控制面的 CLI 完整性。Sunday 的替代方案是再開一個 HTTP 轉發器去摹仿 operator——為了一句話蓋一個服務，荒謬。

## 4. Acceptance

- idle 成員收到後喚醒處理；busy 成員折進當前 run（兩條 drain 路徑各一個 e2e）。
- 訊息在 Web 收件匣/transcript 顯示 sender=`user`，與 Web 發的訊息不可區分。
- 未知 member → 非零 exit + 可糾正錯誤訊息（列出現有成員，比照 `rosterHas` 慣例）；service 未跑 → 同 `swarm ls` 的連線錯誤行為。
- token 鑑權生效（無 token 401）；`--help` / `evva swarm help` 列出新子命令。

## 5. Notes

- 落地後 Sunday 的 persona 迴歸可以寫成 shell 腳本（send → 等 N 秒 → grep event log 斷言行為），是 EX-4 replay harness 之前最便宜的行為驗證迴路。
- 自然的下一步（不在本 RP）：`evva swarm tail <ref> [member]` 串流 event log——配上 send 就是完整的 CLI 對話迴路；先觀察 send 的實際使用再說。
