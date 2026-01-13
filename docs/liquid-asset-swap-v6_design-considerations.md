# PeerSwap v6: Liquid arbitrary-asset swap 設計検討メモ

このドキュメントは、PeerSwap v6 の「Liquid 任意 asset（非BTC peg前提）」swap を導入するにあたり、追加検討が必要な論点を整理するための材料である。

前提:
- v5 互換は不要（protocol v6 固定）。
- Liquid の swap は `network`（liquid/liquid-testnet/liquid-regtest）と、その配下の `asset_id` を指定する。
- LN 側（BTC sats）と onchain 側（asset の base units）を分離し、`ln_amount_sat` と `asset_amount` を明示する（暗黙 1:1 を禁止）。
- Liquid の手数料は LBTC 建てであるため、非LBTC asset の spending で「swap asset から fee を差し引く」ことはできない。

## 1. 現状実装（MVP / A方式）の要約

### 1.1 送受する値（v6）
- `network`: `mainnet|testnet3|testnet4|signet|regtest` または `liquid|liquid-testnet|liquid-regtest`
- `asset_id`:
  - Bitcoin ネットワーク: 空（必須）
  - Liquid ネットワーク: 32-byte hex（big-endian 表記、必須）
- `ln_amount_sat`: LN 側 sats（BTC）
- `asset_amount`: onchain 側の数量（asset base units）

### 1.2 Liquid の fee-reserve（A方式）
Liquid の OpeningTx に、swap asset 出力に加えて「LBTC fee-reserve 出力」を同一 swap script 宛に作る。

- OpeningTx outputs（同一スクリプト宛）:
  - (a) swap asset: `asset_id` / `asset_amount`
  - (b) fee reserve: LBTC / `fee_reserve_sat`
- SpendingTx:
  - inputs: (a) + (b) の 2 inputs
  - outputs:
    - swap asset を受取先に `asset_amount` 全額送金
    - LBTC の fee output を `fee_reserve_sat` 全額で作り、reserve を全額 burn（LBTC change は作らない）

注記:
- `fee_reserve_sat` は `estimated_spending_fee * 2` を採用し、安全側に過大に確保する（MVP）。
- 結果として、Liquid swap の spending fee は「常に fee_reserve を全額 burn」と同義になる（推定精度は後続課題）。

## 2. 「Liquid で asset 未指定なら LBTC fallback」案

### 2.1 要件（案）
ユーザが Liquid swap を選択しているのに `asset_id` を省略した場合、`asset_id = network の LBTC asset id` にフォールバックする。

実装レイヤ案（推奨）:
- wire（p2p message）上は **Liquid の場合は常に `asset_id` を埋める**（受信側 validation を単純化）。
- CLI/RPC の入力で `asset_id` が空なら、送信前に補完する。

追加で UX を揃えるなら:
- `asset_amount` 未指定も許容し、LBTC fallback 時のみ `asset_amount = ln_amount_sat` をデフォルト化（LBTC は peg 前提の “従来体験” を維持）。

### 2.2 利点
- 既存の「lbtc swap」を、`asset_id` を意識せず実行できる。
- LBTC は `asset_amount == ln_amount_sat` が自然で、v6 の “amount 分離” を導入しつつ従来の直感に寄せられる。

### 2.3 リスク / 注意点
- “省略＝LBTC” は意図しない swap を起こし得る（誤爆）。特に今後 UI/自動化が増えるほど危険。
- `asset_id` を必須とする現在の v6 validation と整合させるなら、**省略は CLI/RPC 入口でのみ許容**し、wire では常に埋めるのが安全。

### 2.4 推奨（検討結論のたたき台）
- **CLI/RPC のみで LBTC fallback**（wire は常に `asset_id` を持つ）。
- LBTC fallback 時のみ `asset_amount` もデフォルト化して「最小の入力」で動く体験を維持。
- “明示的に non-LBTC を選びたい” 場合は `--asset_id/--asset_amt` を必須にする（現状維持）。

## 3. asset の価格（レート）をどう扱うべきか

### 3.1 本質: “価値” はプロトコルだけでは決められない
v6 では `ln_amount_sat`（BTC sats）と `asset_amount`（任意 asset units）をユーザが指定するため、実質的な交換レートは

```
implied_price = ln_amount_sat / asset_amount
```

で表現される（unit の取り方は UI 側で調整が必要。asset は decimals が異なる）。

PeerSwap は P2P であり、Boltz/Loop のような “単一プロバイダが見積もりを返す” 形とは異なる。よって「どの価格が妥当か」は、
- 双方のオフチェーン合意（手動）
- もしくは外部参照（oracle/DEX/板）

が不可欠。

### 3.2 既存 swap サービスの参考パターン（一般論）
- Submarine swap / swap provider（例: Boltz 系）:
  - provider がレートと手数料を提示し、ユーザはそれを受け入れる。
  - “見積もり” がプロトコルの中心にある。
  - min swap amount が明確に設定されがち（手数料・運用コスト保護）。
- Lightning Loop 系:
  - “サービス手数料（swap fee）” と “オンチェーン手数料（miner fee）” が分離される。
  - swap fee は sats（オフチェーン）で徴収されることが多い。

### 3.3 PeerSwap v6 に適用するなら（選択肢）
1) **完全手動（MVP継続）**
   - ユーザが `ln_amount_sat` と `asset_amount` を決める。PeerSwap は実行するだけ。
   - Pros: 実装最小・外部依存なし
   - Cons: 誤設定（極端に不利なレート）を防げない

2) **“レート範囲” を policy として設定し、逸脱を拒否**
   - 例: `asset_id` ごとに `min_sat_per_unit` / `max_sat_per_unit` を設定し、受信側が request を reject できるようにする。
   - Pros: 誤爆/攻撃（極端なレート）をある程度防げる
   - Cons: policy 管理が必要、asset decimals/UI 表示が難しい

3) **RFQ（request-for-quote）型プロトコル拡張**
   - request は片側 amount だけ指定し、相手が quote（もう片方 amount）を返す。
   - Pros: “見積もり” をプロトコルに組み込める
   - Cons: 破壊的な設計増、UX/状態/タイムアウト/再送制御が増える（MVP を超える）

## 4. “極端に低い額でも swap が成立してしまう” 問題

### 4.1 何が「ガードできない」のか
`asset_amount` が極端に小さい・価値が不明な asset の場合、
- swap 自体はプロトコル的には成立し得る（双方が合意してしまえば止められない）。
- ただし経済的には、Liquid の opening/spending コスト（LBTC）が value を上回り得る。

重要: PeerSwap は “価値” を知らないため、プロトコルだけで「安すぎる/高すぎる」を判定できない。

### 4.2 ガードの方向性（実装コスト順）
1) **最低金額（`ln_amount_sat`）の引き上げ**
   - 既存の `min_swap_amount_msat` を Liquid asset swap では強めにする。
   - Pros: 簡単・すぐ効く
   - Cons: asset 価値の偏差は防げない

2) **fee-aware な警告/拒否（LBTC swap に限定して強く）**
   - asset が LBTC のときは、`asset_amount` と手数料（opening + reserve）を同一単位で比較できるため、
     - `asset_amount` が fee を大きく下回る場合は reject / warning が可能。
   - non-LBTC では “価値比較” はできないが、少なくとも **LBTC fee コストを表示**して注意喚起は可能。

3) **asset_id ごとの policy（min/max asset_amount, min/max implied price）**
   - 運用側で “この asset は最低これ以上” を持つ。
   - Pros: 実運用に寄る
   - Cons: policy 管理が増える（スコープ増）

## 5. fee がどう変わったか（図示）

ここは “誰が fee を負担するか” が v6 A方式の本質。

### 5.1 旧モデル（LBTC 前提の単一 input）
spending tx の fee を、swap 出力（LBTC）から差し引く。

```
OpeningTx (maker)
  [swap_output]  LBTC: asset_amount  -> swap_script

SpendingTx (taker)
  input: swap_output
  output: receiver gets (asset_amount - fee)
  fee: paid in LBTC (effectively receiver負担)
```

非LBTC asset では「fee を引けない」ため、このモデルは破綻する。

### 5.2 新モデル（v6 / A方式: LBTC fee-reserve）
OpeningTx で “LBTC の reserve” を別出力にしてロックし、spending tx の fee に使う。

```
OpeningTx (maker)
  [swap_output]  ASSET_X: asset_amount  -> swap_script
  [fee_reserve]  LBTC:    reserve_sat   -> swap_script

SpendingTx (taker)
  inputs:
    - swap_output
    - fee_reserve
  outputs:
    - receiver gets ASSET_X: asset_amount (満額)
    - fee output consumes LBTC: reserve_sat (全額 burn / change無し)
```

### 5.3 誰が得/損するか（成功時）
変数:
- `F_open`: opening tx fee（LBTC）
- `R`: fee reserve（LBTC。MVPは概ね `2 * estimate` を全額 burn）

#### Swap-In（initiator=maker）
- maker:
  - 支出: onchain asset `asset_amount` + `F_open` + `R`
  - 収入: LN `ln_amount_sat`
- taker:
  - 支出: LN `ln_amount_sat`
  - 収入: onchain asset `asset_amount`（spending fee を引かれない）

=> spending fee（実質 `R`）は maker 側に寄る。

#### Swap-Out（responder=maker）
- taker（initiator）:
  - 支出: LN `fee_invoice` + LN `ln_amount_sat`
  - 収入: onchain asset `asset_amount`
- maker（responder）:
  - 支出: onchain asset `asset_amount` + `F_open` + `R`
  - 収入: LN `fee_invoice` + LN `ln_amount_sat`

=> `fee_invoice` が `F_open` 相当のみだと、maker は “常に `R` を失う”。
   したがって、`R` は (a) レートで織り込む、(b) premium で補填、(c) fee invoice に含める、のいずれかが必要になり得る。

### 5.4 失敗時（swap invoice 未払い）
Liquid では maker が CSV で refund する際も spending tx を作るため、**reserve は成功/失敗に関係なく burn され得る**（MVP: change が無いので確実に burn）。

=> swap-out は “fee invoice を払わせる” ことで griefing コストを相殺しやすいが、reserve を invoice に含めないと maker が `R` 分だけ常にリスクを負う。

### 5.5 数値例（理解補助）
前提（例）:
- swap 対象: ASSET_X を `asset_amount=1000` units
- LN 側: `ln_amount_sat=10_000` sats
- opening tx fee 見積もり: `F_open=400` sats（LBTC）
- spending tx fee 見積もり: `F_claim_est=300` sats（LBTC）
- reserve: `R = 2 * F_claim_est = 600` sats（MVP は全額 burn）

このとき Liquid 側の “最低限の LBTC コスト感” は:
- maker が opening tx を出すときに `F_open` を支払う（400 sats）
- さらに reserve として `R` をロックしておき、spending/refund で **600 sats を fee として丸ごと失う**

結果:
- swap が成功しても失敗しても、maker はだいたい `F_open + R = 1000` sats 相当の LBTC コストを負担し得る
- taker は onchain の受取数量（ASSET_X）は `asset_amount` 満額で受け取る（fee は引かれない）

直感的な差分:
- 旧モデル（LBTC前提）なら「受取額から fee を差し引く」ため、taker が “受取減” として fee を負担しやすい
- 新モデル（任意asset対応）では「fee は LBTC reserve を燃やす」ため、maker 側に fee 負担が寄る（swap-out は補填設計が必要になりやすい）

## 6. Premium を sats に乗せたい（課題と選択肢）

v5 までの premium は “BTC/LBTC（同一単位）” 前提で、onchain amount に上乗せする/請求する運用が成立しやすかった。
任意 asset では、premium を onchain asset で支払うと意味が崩れるため、**premium を sats（LN側）で支払う**のは自然。

ただし swap-in / swap-out で “誰が誰に支払う premium なのか” が非対称になり得る。

### 6.1 想定される課題
- **原子性（atomicity）**:
  - premium を別支払いにすると、「premium だけ支払われて swap が失敗」などのズレが起き得る。
- **方向性（誰が受け取るか）**:
  - swap-in では taker が LN を支払う側なので、premium を “invoice に上乗せ” すると taker の負担が増え、premium の意味（相手への補填）が逆転する可能性がある。
- **UI/誤設定リスク**:
  - `ln_amount_sat` と `premium_sat` を別で扱うと、ユーザが “何をいくら払うか” を誤解しやすい。
- **チャネル制約**:
  - premium を上乗せすると必要な outbound/inbound が増える（swap 成立率に影響）。

### 6.2 選択肢（方向性のたたき台）
1) **swap-out のみ premium を LN invoice に上乗せ（まずはここから）**
   - swap-out は taker（initiator）が LN invoice を支払うため、maker（responder）への補填として自然。
   - reserve `R` の補填も premium で吸収しやすい。
   - swap-in は premium 無効/別設計と割り切る（段階導入）。

2) **premium を “常に LN sats” として定義し、swap-in は invoice を減額（= taker が少なく払う）**
   - swap-in の premium は “taker（invoice payer）への割引” として表現する。
   - Pros: sats で完結
   - Cons: `ln_amount_sat` の意味がブレる（request の `ln_amount_sat` が “基準値” なのか “実支払額” なのかを再定義する必要）。

3) **premium を別 LN 支払いにする（2-invoice / hold invoice 等）**
   - Pros: 表現力は高い
   - Cons: 実装/状態機械/失敗時処理が増え、atomicity が難しい（MVP を超える）

### 6.3 検討の結論（暫定推奨）
- 「premium を sats に乗せる」目的が **Liquid の reserve `R` を補填する** ことなら、まず swap-out 側に限定して導入するのが現実的。
- swap-in の premium は、割引（invoice 減額）にするか、別支払いにするかを別途設計が必要。

## 7. 次の意思決定ポイント（TODO）
- Liquid で `asset_id` 省略時の LBTC fallback を “CLI/RPCのみ” にするか、“wireでも許す” か。
- swap-out の fee invoice に `R`（reserve）を含めるか（maker の griefing リスク低減）。
- “min swap” をどの単位で守るか:
  - LN sats は既存 policy で守れる
  - asset 側の min/max は policy に入れるか、UI 警告に留めるか
- premium を sats で定義した場合の “誰が誰に払う” を swap-in/out でどう定義するか。
