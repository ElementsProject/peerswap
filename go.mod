module github.com/elementsproject/peerswap

go 1.16

require (
	github.com/btcsuite/btcd v0.22.0-beta.0.20211005184431-e3449998be39
	github.com/btcsuite/btcutil v1.0.3-0.20210527170813-e2ba6805a890
	github.com/btcsuite/btcutil/psbt v1.0.3-0.20210527170813-e2ba6805a890
	github.com/elementsproject/glightning v0.0.0-20220713160855-49a9a8ec3e4d
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.5.0
	github.com/jessevdk/go-flags v1.5.0
	github.com/lightninglabs/protobuf-hex-display v1.4.3-hex-display
	github.com/lightningnetwork/lnd v0.14.1-beta
	github.com/onsi/gomega v1.5.0 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli v1.22.2-0.20191024042601-850de854cda0
	github.com/vulpemventures/go-elements v0.3.7
	github.com/ybbus/jsonrpc v2.1.2+incompatible
	go.etcd.io/bbolt v1.3.6
	golang.org/x/crypto v0.0.0-20211117183948-ae814b36b871 // indirect
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/macaroon.v2 v2.1.0
)

// This fork contains some options we need to reconnect to lnd.
replace github.com/grpc-ecosystem/go-grpc-middleware => github.com/nepet/go-grpc-middleware v1.3.1-0.20220824133300-340e95267339

// Temporary: this server is dead, LND 0.15 no longer refers to this old server address
replace git.schwanenlied.me/yawning/bsaes.git => github.com/Yawning/bsaes v0.0.0-20180720073208-c0276d75487e
