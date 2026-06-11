# EX-6 — Skill 共享生態（space 級技能庫＋ leader 教 worker）

> 狀態：**✅ 已畢業至 [RP-26](../refine-plan/RP-26-shared-skills.md)（Part A+B 全落地
> 2026-06-11）** ｜ 日期：2026-06-10 ｜ 規模：中 ｜ 依賴：無（建立在 RP-10 之上）
>
> 畢業註記：spike 本身未跑——operator 直接揭門進 Part B；本票「最短驗證」三步裡的
> 1、2（loader 疊加 + skill_publish）已是正式實作，第 3 步（friday 發布復盤格式、
> 觀察 reviewer 遵循）轉為 Sunday 真實運營中的回歸劇本。治理對策（web 審視/刪除、
> 發布審計、deny 後手）與 publish 同 commit 上線，垃圾堆積觀察在運營中持續。

## 動機

RP-10 給了 per-member skills（User 經 web 增刪、run 邊界 reload、`WriteSkill`/
`ReloadMemberSkills` 原語都在：`space.go:409`、`supervisor.go:314`）。運營中自然出現
兩個下一步需求：

1. **同一招大家都要**——「怎麼讀 Sunday 的 K 線指標」這種 know-how，現在得逐成員貼一份，
   改版要改 N 份。
2. **leader 想教 worker**——leader 在運營中總結出的流程（「復盤要含這五節」），今天只能
   靠 send_message 口頭講，講完就被 compaction 磨掉；寫成 skill 才是持久的制度化。

## 假設

1. **space 級共享 skill dir**（`<workdir>/agents/shared-skills/`）＋成員 registry 載入時
   疊加（member 同名者優先），就能消掉重複貼份。
2. leader 著作權限可以**窄開**：leader 可寫 shared-skills（等於發布 SOP），但不可寫
   單一成員的私有 skills（避免越權改人設）；User 維持全權。

## 最短驗證（spike）

1. loader 疊加 shared dir（`skill.LoadRegistry` 跑兩個來源合併，十幾行）；
2. 給 leader 一個 `skill_publish {name, description, body}` 工具（薄包 `WriteSkill` 到
   shared dir ＋ 對全員 `ReloadMemberSkills`）；
3. 在 Sunday 讓 friday 把「復盤格式」發布成 shared skill，觀察 reviewer 下一輪復盤
   是否真的載入並遵循。

## 成功訊號

- 一份 shared skill 改版，全員下個 run 邊界生效（不重啟）；
- friday 真的會在合適時機 publish（teamprompt 補一句引導後）；
- reviewer 的產出可見地遵循了被教的格式。

## 風險與邊界

- **治理是核心問題**：RP-10 的紀律是「agent 只載入、不著作」——本探索刻意打開一個口子
  （leader → shared 而已），要觀察會不會出現 skill 垃圾堆積；對策：shared dir 上限
  ＋ web 可審視/刪除（User 終審）。
- prompt cache：skill 目錄變動 → 全員前綴變動 → 一次性 cache miss。低頻可接受
  （RP-10 已接受同款成本）。

## 依賴／去向

無硬依賴。spike 成功 → 立 RP：shared dir 正式化、`skill_publish` 工具、web 的 shared
skills 管理頁三件。
