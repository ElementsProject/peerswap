# peersync パッケージ

`github.com/elementsproject/peerswap/peersync` は、PeerSwap ノード間でピア能力情報を同期するための実装を提供します。DDD 風に分割されていた `application`・`domain`・`infrastructure` 層を統合し、機能別にフラットな構成へと整理されています。

- コアロジックと値オブジェクトはすべて同じパッケージで完結
- 最小限のポート／アダプタ（`Store`, `Lightning`）を実装し、必要であれば `policy.Policy` を渡すだけでアプリケーションサービスが利用可能
- 既存テストは `peersync/*_test.go` にまとまり、モックよりシンプルなスタブ構成

## 主なファイル

- `peer.go`：ピア・アセット・プレミアムレートなどの値オブジェクト群
- `peersync.go`：カスタムメッセージを使った同期ループ (`PeerSync`)
- `ports.go`：外部と接続するための `Store` / `Lightning` ポート定義
- `supervisor.go`：同期ループを監視するスーパーバイザ
- `*_test.go`：スタブを使ったユニットテスト

## 使い方

1. 任意のバイナリ／プラグインから `peersync` パッケージをインポートします。
2. `Store` と `Lightning` を満たすアダプタを用意します。
3. `NewPeerSync` に各アダプタと `PeerID`、必要であれば `policy.Policy` を渡し、`Start` を呼び出します。
4. 必要であれば `Supervisor` を用いて自動再起動やバックオフ制御を追加できます。

```go
package main

import (
	"context"
	"log"

	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/peersync"
)

func main() {
	nodeID, err := peersync.NewPeerID("my-node")
	if err != nil {
		log.Fatalf("invalid peer id: %v", err)
	}

    syncer := peersync.NewPeerSync(
        nodeID,
        &myStore{},             // peersync.Store
        &myLightning{},         // peersync.Lightning (custom messages + peers)
        policy.DefaultPolicy(), // *policy.Policy (nil を渡せば無効化も可能)
    )

	ctx := context.Background()
	if err := syncer.Start(ctx); err != nil {
		log.Fatalf("sync failed: %v", err)
	}
}
```

### ポートインターフェース概要

| インターフェース | 主な役割 |
|------------------|-----------|
| `Store` | ピア状態の永続化・取得・クリーンアップ |
| `Lightning` | Custom message の送受信と接続 peer 列挙（lnd/cln に橋渡し） |

ポリシー判定は `*policy.Policy` をそのまま渡すことで有効になり、`nil` を指定すると全て許可されます。

## テスト

`peersync` 配下の変更は下記で検証できます。

```bash
go test ./peersync/...
```

各テストでは gomock ではなくスタブ実装を用いているため、追加アダプタの例としても参照できます。

## 参考リンク

- ルート README: [`../README.md`](../README.md)
- プロトコル仕様ドラフト: [`../docs/peer-protocol.md`](../docs/peer-protocol.md)
