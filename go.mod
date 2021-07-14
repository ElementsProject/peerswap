module github.com/sputn1ck/peerswap

go 1.16

require (
	github.com/btcsuite/btcd v0.21.0-beta
	github.com/sputn1ck/glightning v0.8.3-0.20210714203706-f4027300c4c2
	github.com/stretchr/testify v1.7.0
	github.com/vulpemventures/go-elements v0.3.0
	go.etcd.io/bbolt v1.3.6
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
)

//replace github.com/sputn1ck/glightning => ../glightning

//replace github.com/vulpemventures/go-elements => ../go-elements
