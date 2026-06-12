# 足球賽果預測管線 — 流程定義(公開資訊)

這份文件對所有成員公開,定義管線的階段、產出物與共同約定。
誰該做什麼的細節,以各自的 system prompt 為準。

## 六階段

| 階段 | 負責人 | 產出物 |
| --- | --- | --- |
| P1 企劃 | planner | `work/data-spec.md` — 資料需求規格書 |
| P2 收集(可平行) | collector-1/2/3 | `data/raw/collector-N-*.csv` + `data/raw/collector-N-notes.md` |
| P3 品管 | qa | `data/dataset.xlsx` + `data/quality-report.md`(關鍵缺口可觸發一輪補抓) |
| P4 分析 | analyst | `work/analysis-report.md` — 特徵分析與模型機率 |
| P5 討論(2~4 輪) | predictor ↔ analyst | `work/discussion-log.md` |
| P6 預測 | predictor | `work/prediction.md` — 最終勝/平/負機率與比分 |

總監 (director) 全程把管線狀態維護在 `work/pipeline-state.md`。

## 收集範圍分工

- **collector-1 戰績與交手**:兩隊近期戰績、FIFA/聯賽排名、歷史交手 (H2H)、
  主客場拆分、進失球統計。
- **collector-2 球員與陣容**:預計先發、關鍵球員近況、傷停名單、陣容身價
  與年齡、教練與慣用陣型。
- **collector-3 情境與市場**:賠率與隱含機率、賽事性質與重要性、場地與天氣、
  賽程密度與疲勞、士氣與新聞面。

## 共同約定

- **檔案格式**:CSV 一律 UTF-8、首列為欄位名、日期 `YYYY-MM-DD`、
  隊名全管線統一拼寫(以 data-spec.md 開頭定義的為準)。
- **資料誠實規則(最重要)**:每張原始表都要有 `source` 欄 —
  網路抓到的標 `web`,用模型內建知識補的標 `model-knowledge`,
  查不到就留空並在 notes 記缺漏。**絕不把編造的數字標成 `web`。**
- **通訊**:工作指派由 director 透過**任務看板**發出(任務描述 = 產出物 +
  驗收標準),驗收通過由 director 結案。成員對外說話的唯一方式是
  `send_message`;直接輸出的文字不會被任何人看到。一般成員只回覆 director;
  唯一例外是 P5 討論階段 predictor ↔ analyst 可直接對話。
- **機率紀律**:任何機率輸出總和必須為 100%,禁止 100%/0% 的鐵口直斷;
  最終預測必附資料品質警語與風險情境。
