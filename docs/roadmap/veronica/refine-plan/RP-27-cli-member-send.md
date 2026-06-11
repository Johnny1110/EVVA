# RP-27 — CLI 成員訊息（`evva swarm send <ref> <member> <text>`）

> 狀態：**✅ 已實作（2026-06-11，feature/RP-27；operator「27 go」即拍板）** ｜ 階段：**第五波** ｜ 優先：**P2（小件）** ｜ 日期：2026-06-11
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

---

## 6. 落地註記（2026-06-11）

比票面更省：§2.2 的「webapi 補一個薄端點」**條件不成立**——Web 從來就走 HTTP
`POST /api/agents/{name}/message?space=<ref>`（不是 WS command），所以零新端點，
CLI 直接打既有路由（`?space=` 本來就收 ref：id 或 name 都行，與 add/vacuum 同模式）。

1. **`evva swarm send <ref> <member> <text|->`** 照 §2 落地：thin authed HTTP client
   （`serviceClient` 同模式：token 從 `~/.evva/service/token`、連線錯誤訊息與 `swarm ls`
   一致）；`-` 讀 stdin；fire-and-forget；不做廣播旗標。**help 文案刻意不廣告 `"all"`**
   ——端點技術上收（Web 信箱語義），但 CLI 維持票面的一對一原語；有記在 code 註解。
   `member` 可寫角色 `leader`（§3.5 ResolveRecipient 既有語義，腳本不用先查成員名）。
2. **訊息 id 回執**：`SendUserMessage` 原本把 `Bus.Send` 回的 uuid 丟掉、端點回 204
   ——改成 `(string, error)` + 端點回 `{"id":…}` 200。Web 的 `req()` 對 200+JSON 與
   204 同樣相容（驗過 api.js，caller 忽略回傳值），FE 零改動。webapi.Backend 介面
   隨之改簽名（fakeBackend 補）。
3. **可糾正錯誤（acceptance §4.3）**：unknown member 的 service 錯誤從裸
   `unknown member %q` 升級為**附 roster 名單**（比照 rosterHas 慣例），保留 "unknown"
   字頭維持 webapi 404 對映；CLI 端 `serviceClient` 本來就把 HTTP body 原樣帶出，
   所以打錯名字直接看到有效清單。這順手讓 Web 信箱也得到同樣的錯誤品質。
4. **drain 雙路 e2e（acceptance §4.1）的誠實盤點**：drain A（idle 喚醒→讀→標讀）
   既有 `TestSendUserMessage_DeliveredAndDrained` 已覆蓋；drain B（busy 折入）**不另寫
   user-message 專屬 e2e**——SendUserMessage 走的就是 `Bus.Send`，與成員互寄同一條路、
   零專屬分支，busy 折入已由 SPRD-1-12 的 drainer 測試族證明；再寫一個只是換 sender
   字串的重複。新增的測試面：service 錯誤清單斷言、webapi 200+{id} 路由斷言、
   CLI client stub 測試（路由/token/回執/stdin/404 清單/空文字不碰網路）。
5. **token 鑑權（acceptance §4.4）**：路由本來就在 `guard` 後面（RP-15），無新工作；
   `--help`/`evva swarm help` 已列 send。
6. Sunday 的 persona 迴歸腳本（§5 note）現在可寫：`evva swarm send sunday analyst-flow
   "..." && sleep 30 && grep ... .vero/events/*.jsonl`。在 user 的 Mac 上做。
