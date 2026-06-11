# Veronica — Explore track（EX-1~6）

> 狀態：**草案 / Draft（待 Johnny 拍板）** ｜ 日期：2026-06-10
> 觸發：[`../health-check-2026-06-10.md`](../health-check-2026-06-10.md) 的探索面結論。
> 與 refine-plan 的分工：**RP = 有明確驗收、直接動工；EX = 先用最短 spike 驗證假設**，
> 成功訊號出現才升級成 RP（屆時在 refine-plan 立號、本檔標註去向）。

每份 EX 的固定格式：**動機 → 假設 → 最短驗證（spike）→ 成功訊號 → 風險與邊界 → 依賴**。

| # | 探索 | 規模 | 依賴 | 一句話 |
| --- | --- | --- | --- | --- |
| [EX-1](EX-1-member-native-memory.md) | 成員原生長期記憶 | 中 | — | 把 Sunday 的 `/api/memory` 模式泛化成 per-member memdir。 |
| [EX-2](EX-2-remote-persona.md) | Remote persona | 大 | — | RemoteController 讓遠端 runtime 入隊（nono 願景）。 |
| [EX-3](EX-3-leader-takeover.md) | Leader 接管 | 中 | RP-14 | leader 卡死＝全隊停擺，先做 operator 一鍵接管。 |
| [EX-4](EX-4-replay-eval-harness.md) | Replay／eval harness | 大 | RP-17 | 重放一天的事件流給新 prompt 版本做 regression eval。 |
| [EX-5](EX-5-wake-jitter.md) | 喚醒 jitter | 小 | — | 整點齊醒的 thundering herd，一行 config 的 spike。 |
| [EX-6](EX-6-skill-sharing.md) | Skill 共享生態 | 中 | — | space 級共享 skill 庫＋ leader 教 worker 技能。 |

**建議節奏**：EX-5 隨時可塞（最小）；EX-1、EX-6 接在 RP-13/14 之後；EX-4 等 RP-17 的
event log；EX-2、EX-3 是方向級，spike 過了再立 RP。
