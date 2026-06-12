# world-football-swarm — 足球賽果預測管線

一個可直接開跑的 **8 成員足球預測 swarm**:一位「總監」(leader) 主持整條
管線,七位專家 (workers) 依六階段接力 — 企劃定義資料廣度、三路平行收集、
品管合併成 Excel、分析師建模、預測專家與分析師多輪辯證後產出最終預測。

```
world-football/
├── evva-swarm.yml              # 隊伍清單(director + 7 位專家)
├── PIPELINE.md                 # 公開流程定義(成員可用 read 查閱)
└── agents/
    ├── main/director/          # 總監 — 接單、指派、驗收、彙報
    └── sub/
        ├── planner/            # 分析企劃師 — 資料需求規格書
        ├── collector-1/        # 收集:戰績與交手
        ├── collector-2/        # 收集:球員與陣容
        ├── collector-3/        # 收集:情境與市場
        ├── qa/                 # 資料品管員 — 清洗合併成 xlsx
        ├── analyst/            # 資料分析師 — 特徵工程與建模
        └── predictor/          # 預測專家 — 多輪辯證後下最終判斷
```

## 管線

```
使用者 ──「預測 6/15 阿根廷 vs 法國」──▶ director
P1 企劃   planner          → work/data-spec.md
P2 收集   collector-1..3   → data/raw/*.csv(三路平行)
P3 品管   qa               → data/dataset.xlsx + data/quality-report.md
                             (關鍵缺口可觸發一輪補抓)
P4 分析   analyst          → work/analysis-report.md(特徵 + Poisson 模型)
P5 討論   predictor ↔ analyst(直接對話 2~4 輪)→ work/discussion-log.md
P6 預測   predictor        → work/prediction.md → director 彙報使用者
```

執行中的進度看 `work/pipeline-state.md`(總監視角);最終成品在
`work/prediction.md`,合併後的資料集在 `data/dataset.xlsx`。

## 怎麼跑

```sh
# 1. 啟動 host(會印出 session token)
evva service start

# 2. 進入這個資料夾並註冊 swarm
cd world-football
evva swarm .
#    → registered space <id>
```

打開 `http://127.0.0.1:8888`,貼上 token,進入 **world-football-swarm** space。

## 開跑

在 Member Console 對 **`director`** 說:

> 預測 2026-06-15 阿根廷 vs 法國(世界盃小組賽)

兩隊、日期、比賽性質講清楚,總監就會直接開跑;資訊不足他會先回問。
接著看管線自己走完:企劃 → 三路收集 → 品管 → 建模 → predictor 與 analyst
的多輪交鋒 → 總監彙報最終預測(勝/平/負機率、最可能比分、信心、風險)。

**觀戰技巧**:看**任務看板**最快 — director 把每階段開成任務(描述 =
產出物 + 驗收標準),親自驗收檔案後才結案,未結案的任務就是管線走到哪了。
想看細節用 **時間軸 / firehose stream** 看全場訊息流;P5 討論階段點開
predictor 或 analyst 的 console 看兩人交鋒。中途有新消息(例如剛公布的
傷停)可隨時丟給 director,他會轉給對的成員。

## 設定

- 每個成員的 `profile.yml` 都 pin 了 DeepSeek model + effort(建立後固定):

  | 成員 | model | effort | 理由 |
  | --- | --- | --- | --- |
  | director | `deepseek-v4-pro` | high | 全管線狀態追蹤 + 階段控管 |
  | planner | `deepseek-v4-pro` | medium | 一次性規格設計 |
  | collector ×3 | `deepseek-v4-pro` | medium | 照規格抓資料 + 整表 |
  | qa | `deepseek-v4-pro` | medium | 清洗合併 + 缺口判斷 |
  | analyst | `deepseek-v4-pro` | high | 特徵工程 + 建模 + 接住挑戰 |
  | predictor | `deepseek-v4-pro` | high | 綜合判斷 + 主導辯證 |

  想省成本可把三位 collector 改 `deepseek-v4-flash`(抓取腳本與資料判讀
  品質會下降)。改完 `profile.yml` 要 **reset** space 才生效。
- `permission_mode: bypass`:收集者與分析師要跑 bash(curl / python),
  品管要寫 xlsx — bypass 讓管線不需逐一核准就能自己跑完;想逐步監督
  就改回 default。
- **`.venv/`**:專案 venv,預裝 openpyxl / pandas / scikit-learn,qa 與
  analyst 跑 python 一律用 `.venv/bin/python3`。系統 python3 是 Homebrew
  管的,`pip3 install`(含 `--user`)會被 PEP 668 擋下 — 別刪 `.venv`,
  刪了就 `python3 -m venv .venv && .venv/bin/pip install openpyxl pandas
  scikit-learn` 重建。
- **資料誠實規則**:每張原始表都有 `source` 欄 — 網路抓的標 `web`、模型
  內建知識補的標 `model-knowledge`,品質報告會如實呈現比例。沒網路也能跑,
  只是時效敏感資料(賠率、傷停、天氣)會被列為缺口、預測信心相應調降。

## 再跑一場

直接對 director 說新的一場即可 — 他會先把上一場的 `work/` 與 `data/` 搬進
`archive/<日期>-<隊A>-vs-<隊B>/` 再開工。想整個重來:

```sh
evva swarm ls                       # 找 space id
evva swarm stop <id>
rm -rf .vero work data archive      # 清掉狀態與所有產出
evva swarm .                        # 重新註冊
```

> ⚠️ 預測僅供研究與娛樂 — 足球比賽的不確定性本來就高,最終報告一定附
> 資料品質警語與風險情境,請勿據此投注。
