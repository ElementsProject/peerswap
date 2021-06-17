module github.com/sputn1ck/sugarmama

go 1.16

require (
	github.com/btcsuite/btcd v0.21.0-beta // indirect
	github.com/btcsuite/btcutil v1.0.2 // indirect
	github.com/sputn1ck/glightning v0.8.2
	github.com/stretchr/testify v1.7.0
	github.com/vulpemventures/go-elements v0.3.0
	go.etcd.io/bbolt v1.3.6
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad // indirect
)

replace github.com/sputn1ck/glightning => ../glightning

replace github.com/vulpemventures/go-elements => ../go-elements
