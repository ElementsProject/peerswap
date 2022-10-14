module github.com/elementsproject/peerswap

go 1.16

require (
	github.com/benbjohnson/clock v1.3.0 // indirect
	github.com/btcsuite/btcd v0.23.2
	github.com/btcsuite/btcd/btcec/v2 v2.2.1
	github.com/btcsuite/btcd/btcutil v1.1.2
	github.com/btcsuite/btcd/btcutil/psbt v1.1.5
	github.com/btcsuite/btcd/chaincfg/chainhash v1.0.1
	github.com/btcsuite/btcwallet v0.16.1 // indirect
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf // indirect
	github.com/coreos/go-systemd/v22 v22.4.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.1.0 // indirect
	github.com/decred/dcrd/lru v1.1.1 // indirect
	github.com/dvyukov/go-fuzz v0.0.0-20220726122315-1d375ef9f9f6 // indirect
	github.com/elementsproject/glightning v0.0.0-20220713160855-49a9a8ec3e4d
	github.com/fergusstrange/embedded-postgres v1.19.0 // indirect
	github.com/form3tech-oss/jwt-go v3.2.5+incompatible // indirect
	github.com/go-errors/errors v1.4.2 // indirect
	github.com/google/btree v1.1.2 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.3.0
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.11.3
	github.com/jackc/pgx/v4 v4.17.2 // indirect
	github.com/jessevdk/go-flags v1.5.0
	github.com/jonboulle/clockwork v0.3.0 // indirect
	github.com/lib/pq v1.10.7 // indirect
	github.com/lightningnetwork/lightning-onion v1.2.0 // indirect
	github.com/lightningnetwork/lnd v0.15.2-beta
	github.com/lightningnetwork/lnd/tor v1.1.0 // indirect
	github.com/ltcsuite/ltcd v0.22.1-beta // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2 // indirect
	github.com/miekg/dns v1.1.50 // indirect
	github.com/prometheus/client_golang v1.13.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/stretchr/testify v1.8.0
	github.com/tmc/grpc-websocket-proxy v0.0.0-20220101234140-673ab2c3ae75 // indirect
	github.com/urfave/cli v1.22.9
	github.com/vulpemventures/go-elements v0.4.0
	github.com/ybbus/jsonrpc v2.1.2+incompatible
	go.etcd.io/bbolt v1.3.6
	go.etcd.io/etcd/server/v3 v3.5.5 // indirect
	go.opentelemetry.io/contrib v1.10.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.36.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.10.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.8.0 // indirect
	go.uber.org/zap v1.23.0 // indirect
	golang.org/x/crypto v0.0.0-20221010152910-d6f0a8c073c2 // indirect
	golang.org/x/net v0.0.0-20221004154528-8021a29435af // indirect
	golang.org/x/sync v0.0.0-20220929204114-8fcdb60fdcc0 // indirect
	golang.org/x/sys v0.0.0-20221010160319-abe0a0adba9c // indirect
	golang.org/x/term v0.0.0-20220919170432-7a66f970e087 // indirect
	golang.org/x/time v0.0.0-20220922220347-f3bd1da661af // indirect
	golang.org/x/tools v0.1.12 // indirect
	google.golang.org/genproto v0.0.0-20221010155953-15ba04fc1c0e // indirect
	google.golang.org/grpc v1.50.0
	google.golang.org/protobuf v1.28.1
	gopkg.in/macaroon-bakery.v2 v2.3.0 // indirect
	gopkg.in/macaroon.v2 v2.1.0
	sigs.k8s.io/yaml v1.3.0 // indirect
)

// This fork contains some options we need to reconnect to lnd.
replace github.com/grpc-ecosystem/go-grpc-middleware => github.com/nepet/go-grpc-middleware v1.3.1-0.20220824133300-340e95267339

// LND-0.15.2 hard codes ancient gocheck and xmlpath references to launchpad.net that require VCS bzr, point at github equivalent until they update their deps
replace launchpad.net/gocheck v0.0.0-20140225173054-000000000087 => github.com/go-check/check v0.0.0-20140225173054-eb6ee6f84d0a
replace launchpad.net/xmlpath v0.0.0-20130614043138-000000000004 => github.com/go-xmlpath/xmlpath v0.0.0-20130614043138-43e5e9adc398

