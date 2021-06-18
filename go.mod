module github.com/sputn1ck/sugarmama

go 1.16

require (
	github.com/btcsuite/btcd v0.21.0-beta
	github.com/sputn1ck/glightning v0.8.3-0.20210618144220-302ac0497e50
	github.com/stretchr/testify v1.7.0
	github.com/vulpemventures/go-elements v0.3.0
	go.etcd.io/bbolt v1.3.6
)

//replace github.com/sputn1ck/glightning => ../glightning

//replace github.com/vulpemventures/go-elements => ../go-elements
