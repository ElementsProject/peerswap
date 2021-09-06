module github.com/sputn1ck/peerswap

go 1.16

require (
	github.com/btcsuite/btcd v0.21.0-beta
	github.com/btcsuite/btcutil v1.0.3-0.20210527170813-e2ba6805a890
	github.com/btcsuite/btcutil/psbt v1.0.3-0.20210527170813-e2ba6805a890
	github.com/sputn1ck/glightning v0.8.3-0.20210906154637-cb7f5eec2d28
	github.com/stretchr/testify v1.7.0
	github.com/vulpemventures/go-elements v0.3.0
	go.etcd.io/bbolt v1.3.6
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
)

// replace github.com/sputn1ck/glightning => ../glightning

//replace github.com/vulpemventures/go-elements => ../go-elements
