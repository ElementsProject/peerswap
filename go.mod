module github.com/sputn1ck/peerswap

go 1.17

require (
	github.com/btcsuite/btcd v0.22.0-beta
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f // indirect
	github.com/btcsuite/btcutil v1.0.3-0.20210527170813-e2ba6805a890
	github.com/btcsuite/btcutil/psbt v1.0.3-0.20210527170813-e2ba6805a890
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/jessevdk/go-flags v1.5.0
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sputn1ck/glightning v0.8.3-0.20210906154637-cb7f5eec2d28
	github.com/stretchr/testify v1.7.0
	github.com/vulpemventures/fastsha256 v0.0.0-20160815193821-637e65642941 // indirect
	github.com/vulpemventures/go-elements v0.3.0
	go.etcd.io/bbolt v1.3.6
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad // indirect
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c // indirect
)

// replace github.com/sputn1ck/glightning => ../glightning

//replace github.com/vulpemventures/go-elements => ../go-elements
