# ExecPlan Standard
#
# このファイルは、ExecPlan（実行計画）の最小標準を定義する。
# ExecPlan は「設計から実装までの意思決定」を追跡し、作業を再現可能にする。

## Goal

- PeerSwap に「Liquidの任意asset（非BTC peg前提）」のswap機能を導入する（v6プロトコル、v5互換なし）。
- swapごとに `asset_id` と `asset_amount`（onchain側数量）と `ln_amount_sat`（LN側数量）を指定し、双方が合意した条件でswapを実行できる。
- LiquidのfeeはLBTC建てである点を前提に、OpeningTx内にLBTCのfee-reserve出力を持つ方式（A方式）でMVPを成立させる。

非目的:

- v5との互換性維持（v6固定）。
- 価格発見・レート計算・自動見積もり（`ln_amount_sat` と `asset_amount` は外部合意前提）。
- Premium（MVPでは無効化/常に0、後続で拡張余地を残す）。

## Scope

変更対象:

- P2Pプロトコル（v6）と実装
  - `swap/messages.go`（request messageの拡張/validation刷新）
  - `swap/service.go`（protocol_version更新、SwapIn/SwapOutの送信ロジック更新）
  - `swap/swap.go`（SwapDataにLN数量とasset数量の分離、getter更新）
  - `swap/actions.go`（invoice金額・min/limitチェック等の参照先を `ln_amount_sat` に統一、asset一致チェック撤廃/刷新）
- Liquidオンチェーン実装（fee-reserve方式）
  - `onchain/liquid.go`（OpeningTxを2出力化: swap asset + LBTC fee-reserve、SpendingTxを2入力化）
  - `onchain/utils.go`（必要ならヘルパ追加）
- Wallet層（複数asset出力/asset指定送金）
  - `wallet/wallet.go`（必要なら複数出力を表現できるI/Fへ更新）
  - `wallet/elementsrpcwallet.go`（複数出力でFund/Sign/Broadcastできるよう拡張）
  - `lwk/lwkwallet.go`（asset引数を無視している箇所の修正、addresseeのasset指定）
  - `lwk/client.go`（`unvalidatedAddressee.Asset` の扱い確認/必要修正）
- RPC/CLI（swapごとasset選択）
  - `peerswaprpc/peerswaprpc.proto`（SwapIn/SwapOut request拡張）
  - `peerswaprpc/server.go`（RPC→SwapServiceの変換）
  - `cmd/peerswaplnd/pscli/main.go`（フラグ追加）
  - `clightning/clightning_commands.go`（フラグ追加）
- ドキュメント
  - `docs/peer-protocol.md`（v6仕様へ更新、fee-reserve方式/非peg前提）
  - `docs/usage.md`（新フラグと運用注意点、LBTC残高必須）
- テスト
  - `swap/*_test.go`（v6フィールド・数量分離・premium無効を反映）
  - `onchain/liquid_test.go`（OpeningTx 2出力 / SpendingTx 2入力の最低限）

変更しないもの:

- peersync のCapability同期仕様（`supported_assets` の表現の全面改修は行わない。必要なら別ExecPlanで対応）。
- 自動レート提示・相場取得・oracle連携。
- GUI/外部ツールの提供。

## Milestones

1) Protocol v6仕様確定 & docs反映
   - 成果: `docs/peer-protocol.md` に v6 message schema（`ln_amount_sat`, `asset_id`, `asset_amount`, `liquid_network`）と必須条件が明記されている。

2) RPC/CLI I/F拡張
   - 成果: `pscli swapin/swapout` と `lightning-cli peerswap-swap-in/out` の両方で swapごとに `asset_id`/`asset_amount`/`ln_amount_sat` を指定できる。

3) SwapDataの数量分離と状態機械の整合
   - 成果: LN invoiceの金額が常に `ln_amount_sat` になり、Liquid onchainのamountが `asset_amount` になる（混線が無い）。

4) Liquid: OpeningTx 2出力化（swap asset + LBTC fee-reserve）
   - 成果: Liquid swap時、OpeningTxに (a) swap asset のHTLC出力 と (b) LBTC fee-reserve HTLC出力 が同一script宛に作られる。

5) Liquid: SpendingTx 2入力化（swap asset + fee-reserve）と検証更新
   - 成果: claim/csv/coop の spending tx が、swap asset を減らさずに受取先へ送金でき、feeはfee-reserve LBTCで支払われる。

6) エンドツーエンド検証（手動/統合）
   - 成果: regtest/liquid-regtest（または testnet）で、非peg assetのswap-in/outが1回ずつ完走する（ログ・txid・swap stateで確認）。

## Tests

Integration（優先）:

- regtest 2ノード（CLN-CLN / LND-LNDのどちらか）+ Liquid-regtest（elementsd もしくは LWK+electrum）を立てる。
- 事前に両者のLiquid walletに
  - swap対象asset（例: assetX）を必要量
  - fee用のLBTCを十分量
  を入金する。
- 代表操作:
  - swap-out: `ln_amount_sat` と `asset_amount` を指定してswap開始→OpeningTx生成→confirm→invoice支払→claim→swap完了
  - swap-in: 同様に完走
- 観測点:
  - `peerswap-listswaps` / `pscli listswaps` のstate遷移
  - OpeningTxに2出力があること（swap assetとLBTC）
  - ClaimTxがswap assetを減らさずに受取先へ送金していること

Unit/Component（補助）:

- `onchain/liquid_test.go`:
  - OpeningTx生成の戻り値が「swap asset vout」を指すこと
  - txHex解析でswap asset出力とLBTC出力が存在すること
  - spending tx生成が2入力であること（最低限）
- `swap/*_test.go`:
  - request validation（Liquid: network+asset必須、BTC: networkのみ等）
  - invoice金額が `ln_amount_sat` に一致すること

## Decisions / Risks

主要な判断:

- v5互換なし（`protocol_version=6` 固定）: 実装を単純化し、旧仕様の曖昧さ（asset/network排他、amount混線）を解消する。
- 非BTC peg前提: LN(BTC)の金額とLiquid(asset)の金額を分離（`ln_amount_sat` と `asset_amount`）して、暗黙1:1を禁止する。
- fee-reserve方式（A方式）:
  - Liquid feeはLBTC建てであり、非LBTC assetのspendingで「swap assetからfee差引」は破綻する。
  - OpeningTxでLBTC fee-reserveを同一swap scriptにロックし、Claim/CsvでそのLBTCをfeeとして消費することで、追加のwallet入力を不要にする。
- fee-reserveの算出:
  - `GetFee(推定サイズ) * 2` を上限として、その全額をfeeとして燃やす（MVP）。

既知リスクと緩和策:

- fee-reserve過大（ユーザ体験/コスト増）:
  - MVPは安全側。後続で「推定精度改善」「余剰を返す（お釣り出力）」を検討。
- wallet実装差（elementsd vs LWK）:
  - LWKは現状 `asset` 引数が無視されているため必ず修正する。`lwk/lwkwallet.go` でaddresseeにassetを設定。
- 監視対象voutの扱い:
  - `OpeningTxBroadcastedMessage.ScriptOut` は swap asset出力voutを維持し、既存watcherを壊さない。
  - LBTC fee-reserve出力voutはspending作成時にOpeningTxHexから再探索する。
- Premium無効化による既存機能の後退:
  - v6移行は破壊的変更。MVPではpremiumを外し、後続で `premium_ln_sat` 等の明示フィールドで復活させる。

## Progress

- 2026-01-12: 要件合意
  - v5互換不要、非BTC peg前提、swapごとasset選択を必須化。
  - Liquid feeはOpeningTx内にLBTC fee-reserveを持つ方式（A方式）で進める。
  - fee-reserve算出は `GetFee(推定サイズ) * 2` を上限、全額をfeeとして消費（MVP）で合意。
