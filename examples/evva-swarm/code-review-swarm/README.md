# code-review-swarm — 程式碼審查小組

一個可直接開工的 **5 成員程式碼審查 swarm**:一位「審查長」(leader) 主持,
三位審查者 (workers) 從**正確性 / 安全 / 品質**三個面向平行審查同一段程式碼,
再由一位**對抗驗證者**逐條重讀程式碼、以推翻為目標驗證每條發現 — 推翻不了
的才進最終報告。這個形狀演示的是 swarm 的「**平行扇出 + 對抗驗證**」模式:
發現靠廣度,可信度靠對抗。

```
code-review-swarm/
├── evva-swarm.yml              # 隊伍清單(lead + 3 reviewer + verifier)
├── REVIEW-GUIDE.md             # 公開面向定義與發現格式(成員可用 read 查閱)
└── agents/
    ├── main/lead/               # 審查長 — 接單、指派、去重、彙整報告
    └── sub/
        ├── reviewer-correctness/  # 正確性:邏輯、邊界、並發、錯誤處理
        ├── reviewer-security/     # 安全:注入、機密、權限、依賴
        ├── reviewer-quality/      # 品質:重複、複雜度、效能、可讀性
        └── verifier/              # 對抗驗證 — 逐條嘗試推翻,守住報告品質
```

## 流程

```
使用者 ──「審查 <repo> 的 <範圍>」──▶ lead
P1 審查   reviewer ×3(平行)→ review/findings-{correctness,security,quality}.md
          lead 去重(同位置同根因合併)
P2 驗證   verifier 逐條重讀程式碼 → review/verdicts.md
                                    (confirmed / refuted / uncertain)
P3 報告   lead 彙整 → review/report.md → 彙報使用者
```

執行中的進度看 `review/review-state.md`(審查長視角);最終成品在
`review/report.md` — confirmed 按嚴重度排序,refuted 也會列出(讓你知道
哪些「看起來像問題」其實不是)。

## 怎麼跑

```sh
# 1. 啟動 host(會印出 session token)
evva service start

# 2. 進入這個資料夾並註冊 swarm
cd code-review-swarm
evva swarm .
#    → registered space <id>
```

打開 `http://127.0.0.1:8888`,貼上 token,進入 **code-review-swarm** space。

## 開工

在 Member Console 對 **`lead`** 說(repo 用絕對路徑,範圍講清楚):

> 審查 /Users/me/proj/foo 的 feature-x 分支對 main 的 diff

範圍可以是一段 diff、一個分支、一個目錄、或一個小專案。lead 會先用
`git diff --stat` 確認範圍取得到、大小合理(太大會建議你切輪),然後開跑:
三路平行審查 → 去重 → verifier 逐條對抗驗證 → 報告。

**觀戰技巧**:看**任務看板**最快 — P1 三張任務平行跑,P2 一張驗證任務,
未結案的任務就是審查走到哪了。想看 verifier 怎麼推翻一條發現,點開它的
console 看它重讀了哪些檔案。中途想追加範圍可隨時丟給 lead。

## 設定

- 每個成員的 `profile.yml` 都 pin 了 DeepSeek model + effort(建立後固定):

  | 成員 | model | effort | 理由 |
  | --- | --- | --- | --- |
  | lead | `deepseek-v4-pro` | high | 階段控管 + 去重 + 彙整 |
  | reviewer-correctness | `deepseek-v4-pro` | high | 追控制流與並發最吃推理 |
  | reviewer-security | `deepseek-v4-pro` | medium | 沿輸入路徑追查 + 快篩確認 |
  | reviewer-quality | `deepseek-v4-pro` | medium | 模式辨識為主 |
  | verifier | `deepseek-v4-pro` | high | 逐條重讀 + 對抗推理 |

  改完 `profile.yml` 要 **reset** space 才生效。
- **整個 swarm 對目標 repo 一律唯讀**:成員的 `bash` 只跑 `git diff` /
  `git log` / `git blame` 這類唯讀指令,所有產出只寫進本專案的 `review/`。
- `permission_mode: bypass`:讓整場審查不需逐一核准就自己跑完;對目標
  repo 不放心就在 `evva-swarm.yml` 改回 `default` 逐步監督。
- 想調整面向(例如加一路「測試覆蓋審查」):在 `agents/sub/` 加一個成員、
  `evva-swarm.yml` 的 `workers` 加一行、`REVIEW-GUIDE.md` 加一節,
  再 reset space。

## 再審一輪

直接對 lead 說新的範圍即可 — 他會先把上一輪的 `review/` 搬進
`review/archive-<日期>-<範圍>/` 再開工。想整個重來:

```sh
evva swarm ls                       # 找 space id
evva swarm stop <id>
rm -rf .vero review                 # 清掉狀態與所有產出
evva swarm .                        # 重新註冊
```

> ⚠️ 審查結論僅供參考 — verifier 擋得掉誤報,擋不掉漏報;merge 前的最終
> 判斷仍在人。
