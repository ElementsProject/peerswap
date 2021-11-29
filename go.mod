module github.com/sputn1ck/peerswap

go 1.16

require (
	github.com/btcsuite/btcd v0.22.0-beta.0.20211005184431-e3449998be39
	github.com/btcsuite/btcutil v1.0.3-0.20210527170813-e2ba6805a890
	github.com/btcsuite/btcutil/psbt v1.0.3-0.20210527170813-e2ba6805a890
	github.com/jessevdk/go-flags v1.5.0
	github.com/lightningnetwork/lnd v0.13.0-beta.rc5.0.20211025212410-0a3bc3ee3dd4
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/onsi/gomega v1.5.0 // indirect
	github.com/sputn1ck/glightning v0.8.3-0.20211026092153-719e345cf2cd
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli v1.22.5 // indirect
	github.com/vulpemventures/go-elements v0.3.4
	github.com/ybbus/jsonrpc v2.1.2+incompatible
	go.etcd.io/bbolt v1.3.6
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/macaroon.v2 v2.1.0
)

//replace github.com/sputn1ck/glightning => ../glightning

// replace github.com/vulpemventures/go-elements => ../go-elements
