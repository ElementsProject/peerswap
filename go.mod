module github.com/sputn1ck/sugarmama

go 1.16

require (
	github.com/btcsuite/btcd v0.21.0-beta
	github.com/niftynei/glightning v0.8.2 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/vulpemventures/go-elements v0.3.0
	go.etcd.io/bbolt v1.3.6 // indirect

)

replace github.com/niftynei/glightning => ../glightning
