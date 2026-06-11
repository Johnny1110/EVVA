# RP-26 — Space 級共享 skills（EX-6 的畢業票：Part A 共享目錄先行）

> 狀態：**草案 / Draft（待 Johnny 拍板）** ｜ 階段：**第五波** ｜ 優先：**P2（Part A 可隨時動工；Part B 看 EX-6 spike）** ｜ 日期：2026-06-11
> 觸發：Sunday swarm 重整。具體重複實例：`query-sunday` SKILL.md 在 risk-monitor 與 reviewer 各貼一份——查 Sunday 帳戶端點的 know-how 是全隊共用知識，現在改版要改兩處（隊伍再長就是 N 處）。
> 關聯：**[EX-6](../explore/EX-6-skill-sharing.md)（parent——本文是它「spike 成功 → 立 RP」的去向；Part B 治理問題以 spike 結論為準）**、[RP-10](RP-10-agent-skills-injection-and-web-mgmt.md)（per-member skills 地基；`WriteSkill`/`ReloadMemberSkills` 原語）、[RP-10A](RP-10A-subtickets.md)（「agent 只載入、不著作」紀律）
> 請求者：Sunday。**無 Sunday-specific code。**

---

## 1. Problem（observed）

skills 目前嚴格 per-member（`agents/{main,sub}/<name>/skills/`）。兩個成長痛（EX-6 §動機 + Sunday 實例）：

1. **共用 know-how 重複貼份**：`query-sunday` ×2 只是開始——「怎麼讀 K 線指標端點」「PRD 開票格式」天然全隊適用；逐成員複製 = 改版漂移（兩份已開始各自演化）。
2. **leader 的制度化管道缺失**：leader 總結的流程只能 send_message 口傳，compaction 一過就消失；skill 才是持久載體，但 RP-10 紀律（正確地）禁止 agent 著作 per-member skill。

## 2. Proposal（兩段切，A 不等 B）

**Part A — 共享目錄（機械、可直接動工）**：

1. 新增 `<workdir>/agents/skills/`（space 級共享）；`Loader.Build`（`internal/swarm/agentdef/loader.go:104` 的 `skill.LoadRegistry` 處）改為**兩源合併**：member 私有 + space 共享，**同名時 member 版優先**（局部覆寫全域，與一般 config 疊加直覺一致）。
2. 提示詞的 skill 清單（RP-10-1 強制注入）自然帶出共享技能——零額外注入面。
3. 熱更新沿 RP-10 的 run-邊界 reload；`evva swarm .` 重註冊全量生效。

**Part B — `skill_publish`（leader 著作窄口，gated on EX-6 spike）**：

- leader 工具 `skill_publish {name, description, body}` → 薄包 `WriteSkill`（`space.go:409`）寫進 shared dir + 對全員 `ReloadMemberSkills`（`supervisor.go:314`）。
- 治理邊界照 EX-6：leader **只能寫 shared**、不能寫成員私有 dir（不越權改人設）；User 在 Web 終審可刪（FE-7）。
- **EX-6 spike 的觀察點（skill 垃圾堆積與否）是 Part B 的開工門檻**——spike 沒跑或結論負面，Part B 不動，Part A 照常有效。

## 3. Why evva（not Sunday）

skill 的載入與合併是 agentdef loader 的職權。Sunday 端唯一替代是繼續複製貼上 + 人肉同步——這正是要消滅的東西。

## 4. Acceptance

- 放一份 skill 進 `agents/skills/` → 全員 prompt 的 skill 清單可見、`skill` 工具可載入。
- member 私有同名 skill 蓋過共享版（載入優先序測試）。
- 無 `agents/skills/` 目錄的 space 行為與今日完全相同。
- （Part B）leader `skill_publish` → 下個 run 邊界全員生效；leader 對成員私有 skills 目錄無寫路徑；發布事件入 event log。
- Sunday 回歸：`query-sunday` 收斂為 shared 單份，risk-monitor / reviewer 行為不退化。

## 5. Notes

- Part A 對 RP-10-3 紀律零衝突：共享 dir 仍是 User 著作（放檔案進去）、agent 載入。
- prompt cache：共享 skill 變動 → 全員前綴一次性 miss——RP-10 已接受同款成本，低頻可控。
- 命名建議避開 `agents/main|sub` 的角色語義：`agents/skills/` 平放即可，loader 用「不是 main/ 不是 sub/」即可辨識，無 schema 變更。
