package swap

import (
	"github.com/sputn1ck/liquid-loop/lightning"
	"github.com/vulpemventures/go-elements/transaction"
)

type TxWatcher interface {
	GetCommitmentTx(txId string) (*transaction.Transaction, error)
}

type TxClaimer interface {
	ClaimCommitmentTx() (string, error)
}

type TxCreator interface {
	CreateCommitmentTx(takerPubkeyHash, makerPubkeyHash, pHash string, amount uint64) (txId string, fee int64, err error)
}

type LightningClient interface {
	GetPayreq(amount uint64, preImage, pHash string) (string, error)
	DecodePayreq(payreq string) (*lightning.Invoice, error)
	PayInvoice(payreq string) (preimage string, err error)
}
type SwapHandler struct {
	pc lightning.PeerCommunicator
}

func (sh *SwapHandler) Start() error {
	err := sh.pc.AddMessageHandler(sh.OnMessageReceived)
	if err != nil {
		return err
	}
	return nil
}

func (sh *SwapHandler) OnMessageReceived(peerId string, message string) error {
	return nil
}
