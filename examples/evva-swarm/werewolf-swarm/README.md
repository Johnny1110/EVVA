# werewolf-swarm — 12 人狼人殺

一個可直接開局的 **13 成員狼人殺 swarm**:一位「上帝」(leader) 主持整場
12 人標準局(預女獵守),12 位玩家 (workers) 參與。

**身分每局開局抽籤** — 玩家的 `system_prompt.md` 不含任何預設身分,只有
板子配置與各角色玩法(公開知識)。上帝收到「開始遊戲」後用 `bash` 跑真隨機
洗牌(4 狼人、預言家、女巫、獵人、守衛、4 平民 → 座位 1~12),把結果記進
`game/game-state.md`,再逐人私訊發牌。每局重抽,上一局的身分作廢。
完整規則見 [`RULES.md`](RULES.md)。

```
werewolf-swarm/
├── evva-swarm.yml              # 隊伍清單(god + player-1..12)
├── RULES.md                    # 公開規則(玩家可用 read 查閱)
└── agents/
    ├── main/god/               # 上帝 — 抽籤發牌、主持、裁判、判定勝負
    └── sub/
        └── player-1 .. 12/     # 玩家 — 無預設身分,開局聽上帝發牌
```

> 想知道本局誰是狼?看 `game/game-state.md`(上帝視角,劇透注意)。

## 怎麼跑

```sh
# 1. 啟動 host(會印出 session token)
evva service start

# 2. 進入這個資料夾並註冊 swarm
cd werewolf-swarm
evva swarm .
#    → registered space <id>
```

打開 `http://127.0.0.1:8888`,貼上 token,進入 **werewolf-swarm** space。

## 開局

在 Member Console 對 **`god`** 說:

> 開始遊戲

接著看上帝跑完整個流程:

- **開局**:用 `bash` 真隨機抽籤 → 逐人私訊發牌(狼人會收到隊友名單)→
  公告規則(不透露抽籤結果)。
- **夜晚**:守衛 → 狼人(逐狼收刀口、多數決)→ 預言家(查驗)→ 女巫(用藥)
  全部走私訊;打開任一玩家的 console 可以看到 ta 與上帝的夜間對話。
- **白天**:宣布死訊 → 遺言 → 按座位輪流發言(發言會附前面所有人的逐字稿)
  → 投票 → 公布票型 → 放逐。
- 上帝把牌局狀態維護在 `game/game-state.md`(藥水、守衛紀錄、查驗、票型),
  終局時公布所有身分與全場回顧。

**觀戰技巧**:用 **時間軸 / firehose stream** 看全場訊息流;點 roster 裡的
玩家直接進入 ta 的視角。想知道內幕就看 `game/game-state.md`(上帝視角,劇透)。

你也可以隨時私訊任何玩家聊兩句 — 訊息走同一條 bus,不會中斷牌局,但請避免
洩漏你從上帝視角看到的資訊。

## 設定

- 每個成員的 `profile.yml` 都 pin 了 DeepSeek model + effort(建立後固定)。
  身分是抽籤的 — 任何座位都可能抽到狼人或預言家,所以玩家統一規格:

  | 成員 | model | effort | 理由 |
  | --- | --- | --- | --- |
  | 上帝 | `deepseek-v4-pro` | high | 全局狀態追蹤 + 嚴格回合控管 |
  | 玩家 ×12 | `deepseek-v4-pro` | medium | 任何人都可能抽到吃推理的牌 |

  想省成本可把玩家全改 `deepseek-v4-flash`(說謊與推理品質會下降)。
  改完 `profile.yml` 要 **reset** space 才生效。
- `permission_mode: bypass`:純對話遊戲,只有上帝會寫 `game/` 狀態檔,
  不需要逐一核准。

## 重開一局

```sh
evva swarm ls                       # 找 space id
evva swarm stop <id>
rm -rf .vero game                   # 清掉牌局狀態與 ledger
evva swarm .                        # 重新註冊開新局
```

> 身分是每局抽籤的,重開即重抽 — 不需要改任何檔案。想換板子(角色構成),
> 改 `agents/main/god/system_prompt.md` 的洗牌指令與 `RULES.md` 的板子表。
