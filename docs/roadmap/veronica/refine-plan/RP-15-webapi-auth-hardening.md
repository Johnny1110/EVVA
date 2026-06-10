# RP-15 — WebAPI 認證硬化（兌現 minted-token TODO）

> 狀態：**✅ 已完成（2026-06-10，feature/RP-15-webapi-auth）** ｜ 日期：2026-06-10 ｜ 波次：**第四波（運營硬化）** ｜ 優先：P1
> 觸發：2026-06-10 健康檢查——`service.go:58` 自己標的 TODO（「非 loopback 暴露前要換
> minted token」）至今未兌現；webhook endpoint 刻意免 token（RP-9 的 loopback 信任）。
> 本機單人用沒問題；**任何 reverse-proxy / 區網暴露前，本 RP 是硬前置。**
> 上層：[`../health-check-2026-06-10.md`](../health-check-2026-06-10.md) ｜ 前文：[RP-9](RP-9-external-event-webhook.md)（免 token 是當時明示的測試期 tradeoff）

---

## 1. 現況盤點（file:line 證據）

| # | 事實 | 位置 | 意義 |
| --- | --- | --- | --- |
| S1 | 預設綁 `127.0.0.1:8888`（invariant #6 的安全基線） | `service/service.go:49-52` | ✅ loopback 邊界是今天唯一的防線 |
| S2 | guard token 存在但屬「test-convenience tradeoff」 | `service/service.go:57,228` | ⚠️ 弱（可預期/可繞） |
| S3 | webhook `POST /api/swarm/{id}/event` 刻意不掛 guard | `webapi/api.go:317-323` | ⚠️ loopback 上任何進程可喚醒 leader |
| S4 | 真 TODO：minted token | `service/service.go:58` | 本 RP 的存在理由 |

## 2. 設計方向

1. **Minted token**：service 啟動時生成（`common.GenUUID` 級隨機），寫
   `~/.evva/service/token`（0600）；CLI（`evva swarm …`）與 FE 自動讀取——loopback
   單機體驗**不變**（人不用打 token）。
2. **非 loopback 顯式 opt-in**：`--addr` 非 127.0.0.1 時拒絕啟動，除非同時給
   `--allow-remote`；此時所有 endpoint（含 WS、含靜態頁之外的 API）強制 token。
3. **Webhook secret（向後相容）**：`settings.webhook_secret` 設了才驗（header
   `X-Evva-Webhook-Secret` 或 HMAC-SHA256 簽 body 二選一，傾向先做 shared-secret
   header——Sunday `events.post` 加一個 header 即可）；不設則維持 loopback 信任。
4. 文件：user-guide 加「把工作站掛到區網/外網」一節，明示威脅模型（agents hold only
   HTTP；token 即全權——等同 operator）。

## 3. 驗收（DoD）

1. loopback 預設流程零變化（CLI/FE 不用人工配 token）。
2. `--addr 0.0.0.0:8888` 無 `--allow-remote` → 啟動即拒絕；有 → 無 token 的請求一律 401。
3. 設定 `webhook_secret` 後，無/錯 secret 的 event POST 一律 401，Sunday 帶 secret 正常；
   未設定時行為與現狀一致。
4. token 檔權限 0600；輪換 = 重啟（v1 不做線上輪換）。

## 4. 非目標

- 多使用者/RBAC、TLS 終結（交給 reverse proxy）、OAuth——單 operator 模型不變。

## 5. 實作記錄與偏離（2026-06-10）

落地內容與 §2 設計的差異，按條目對照：

1. **Minted token**：`service.New` 直接 `common.GenUUID()`，`DefaultToken = "root"`
   連同 `service.go:58` 的 TODO 一併刪除。token 檔（0600）與 CLI 讀檔其實
   **SPRD-1-9 早就做好了**（`serviceRun` 寫 `<AppHome>/service/token`、
   `serviceClient` 讀同一檔）——真正的缺口只有「root 可預測」本身。
2. **FE 自動讀取的機制**＝新端點 `GET /api/auth/bootstrap`（PRD 只說「自動讀取」
   沒定機制）：僅當 **service 非 remote 模式 且 TCP 對端是 loopback** 才回
   `{token}`，否則一律 404（不暴露存在）。`--allow-remote` 下整個端點消失——
   反向代理會把所有人洗成 loopback 對端，這是防 token 外洩的硬閘。FE 端
   `session.bootstrap()` 開頁自動登入；token 每次重啟輪換，所以 Landing 頁對
   401 會自動丟棄舊 token 再 bootstrap 一次（stale-token 自癒）。
3. **--allow-remote**：旗標＋`EVVA_SERVICE_ALLOW_REMOTE=1`（傳給 daemon 子進程）。
   雙重執法：CLI 父進程 fail-fast（訊息出現在你的終端，不是 daemon log），
   `Service.Listen` 再擋一次（覆蓋 embedder 路徑）。判定函數 `IsLoopbackAddr`
   匯出：wildcard（`:8888` / `0.0.0.0` / `[::]`）一律視為非 loopback。
4. **Webhook secret**：照 PRD 傾向只做 shared-secret header
   （`X-Evva-Webhook-Secret`，constant-time 比對），HMAC 留待真需求。
   **比 PRD 多加一條規則**：未設 secret 的 space 對「非 loopback 對端」直接 401
   ——否則 `--allow-remote` 一開，webhook 就是裸的喚醒端點。本機呼叫者行為與
   RP-9 完全一致（DoD#3 的「未設定＝現狀」只對 loopback 成立，遠端收緊是有意的）。
5. **DoD#2 澄清**：「remote 模式所有 endpoint 強制 token」其實 pre-RP-15 就成立
   （guard 本來就無條件）；真正的洞是可預測 token＋免驗 webhook，兩者都已封。
6. 測試：service 層（mint 唯一性、Listen 拒綁、secret 四象限、remote-needs-secret）、
   webapi 層（bootstrap 三閘門、401 對映、header/loopback 上報）、manifest
   round-trip；`go test ./...` 與 `-race` 全綠。文件：user-guide zh/en §10 新增
   「暴露到本機之外」與「webhook + secret」兩節（RP-9 的 webhook 用法此前從未
   進過 user guide，一併補上）、§5.2 manifest 範例、§11 CLI/環境變數表。
