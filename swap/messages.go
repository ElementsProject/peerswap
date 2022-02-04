package swap

import (
	"encoding/json"

	"github.com/sputn1ck/peerswap/messages"
)

type SwapInRequestMessage struct {
	// ProtocolVersion is the version of the PeerSwap peer protocol the sending
	// node uses.
	ProtocolVersion uint8 `json:"protocol_version"`
	// SwapId is a randomly generated 32 byte string that must be kept the same
	// through the whole process of a swap and serves as an identifier for a
	// specific swap.
	SwapId *SwapId `json:"swap_id"`
	// Network is the desired on-chain network to use. This can be:
	// Bitcoin: mainnet, testnet, signet, regtest
	// Liquid: The field is left blank as the asset id also defines the bitcoinNetwork.
	Network string `json:"network"`
	// Asset is the desired on-chain asset to use. This can be:
	// Bitcoin: The field is left blank.
	// Liquid: The asset id of the networks Bitcoin asset.
	Asset string `json:"asset"`
	// Scid is the short channel id in human readable format, defined by BOLT#7
	// with x as separator, e.g. 539268x845x1.
	Scid string `json:"scid"`
	// Amount is The amount in Sats that is asked for.
	Amount uint64 `json:"amount"`
	Pubkey string `json:"pubkey"`
}

func (s SwapInRequestMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPINREQUEST
}

func (s SwapInRequestMessage) Validate() error {
	panic("implement me")
}

// SwapInAgreementMessage is the response by the swap-in peer if he accepts the
// swap.
type SwapInAgreementMessage struct {
	// ProtocolVersion is the version of the PeerSwap peer protocol the sending
	// node uses.
	ProtocolVersion uint8 `json:"protocol_version"`
	// SwapId is a randomly generated 32 byte string that must be kept the same
	// through the whole process of a swap and serves as an identifier for a
	// specific swap.
	SwapId *SwapId `json:"swap_id"`
	// Pubkey is a 33 byte compressed public key used for the spending paths in
	// the opening_transaction.
	Pubkey string `json:"pubkey"`
	// Premium is a compensation in Sats that the swap partner wants to be payed
	// in order to participate in the swap.
	Premium uint64 `json:"premium"`
}

func (s SwapInAgreementMessage) Validate() error {
	panic("implement me")
}

func (s SwapInAgreementMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPINAGREEMENT
}

// SwapOutRequestMessage gets send when a peer wants to start a new swap.
type SwapOutRequestMessage struct {
	// ProtocolVersion is the version of the PeerSwap peer protocol the sending
	// node uses.
	ProtocolVersion uint8 `json:"protocol_version"`
	// SwapId is a randomly generated 32 byte string that must be kept the same
	// through the whole process of a swap and serves as an identifier for a
	// specific swap.
	SwapId *SwapId `json:"swap_id"`
	// Asset is the desired on-chain asset to use. This can be:
	// Bitcoin: The field is left blank.
	// Liquid: The asset id of the networks Bitcoin asset.
	Asset string `json:"asset"`
	// Network is the desired on-chain network to use. This can be:
	// Bitcoin: mainnet, testnet, signet, regtest
	// Liquid: The field is left blank as the asset id also defines the bitcoinNetwork.
	Network string `json:"network"`
	// Scid is the short channel id in human readable format, defined by BOLT#7
	// with x as separator, e.g. 539268x845x1.
	Scid string `json:"scid"`
	// Amount is The amount in Sats that is asked for.
	Amount uint64 `json:"amount"`
	// Pubkey is a 33 byte compressed public key used for the spending paths in
	// the opening_transaction.
	Pubkey string `json:"pubkey"`
}

func (s SwapOutRequestMessage) Validate() error {
	panic("implement me")
}

func (s SwapOutRequestMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPOUTREQUEST
}

// SwapOutAgreementMessage is the response by the swap-out peer if he accepts
// the swap it contains an Invoice that the swap-out initiator must pay.
type SwapOutAgreementMessage struct {
	// ProtocolVersion is the version of the PeerSwap peer protocol the sending
	// node uses.
	ProtocolVersion uint8 `json:"protocol_version"`
	// SwapId is a randomly generated 32 byte string that must be kept the same
	// through the whole process of a swap and serves as an identifier for a
	// specific swap.
	SwapId *SwapId `json:"swap_id"`
	// Pubkey is a 33 byte compressed public key used for the spending paths in
	// the opening_transaction.
	Pubkey string `json:"pubkey"`
	// Payreq is a BOLT#11 invoice with an amount that covers the fee expenses
	// for the on-chain transactions.
	Payreq string
}

func (s SwapOutAgreementMessage) Validate() error {
	panic("implement me")
}

func (s SwapOutAgreementMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_SWAPOUTAGREEMENT
}

// OpeningTxBroadcastedMessage is the message sent by the creator of the opening
// tx.
type OpeningTxBroadcastedMessage struct {
	// SwapId is the unique identifier of the swap.
	SwapId *SwapId `json:"swap_id"`
	// Payreq is the invoice as described in BOLT#11 that the responder is
	// requested to pay.
	Payreq string `json:"payreq"`
	// TxId is the transaction id of the opening_transaction broadcasted by the
	// initiator.
	TxId string `json:"tx_id"`
	// ScriptOut is the transaction output that contains the opening_transaction
	// output script for the swap.
	ScriptOut uint32 `json:"script_out"`
	// BlindingKey:
	// Bitcoin: Blank.
	// Liquid BitcoinNetwork: Is the 32 byte blinding key to un-blind the outputs of
	//the opening_transaction.
	BlindingKey string `json:"blinding_key"`
}

func (s OpeningTxBroadcastedMessage) Validate(chain string) error {
	if chain == l_btc_chain {
		// check blinding key
	}
	return nil
}

func (t OpeningTxBroadcastedMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_OPENINGTXBROADCASTED
}

// CancelMessage is the message sent by a peer if he wants to / has to cancel
// the swap
type CancelMessage struct {
	// SwapId is the unique identifier of the swap.
	SwapId *SwapId `json:"swap_id"`
	// Message is a hint to why the swap was canceled.
	Message string `json:"message"`
}

func (e CancelMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_CANCELED
}

func (s CancelMessage) Validate() error {
	panic("implement me")
}

// CoopCloseMessage is the message sent by the transaction taker if he wants to
// cancel the swap, but allow the maker a quick close
type CoopCloseMessage struct {
	// SwapId is the unique identifier of the swap.
	SwapId *SwapId `json:"swap_id"`
	// Message is a hint to why the swap was canceled.
	Message string `json:"message"`
	// privkey is the private key to the pubkey that is used to build the opening_transaction.
	Privkey string `json:"privkey"`
}

func (c CoopCloseMessage) MessageType() messages.MessageType {
	return messages.MESSAGETYPE_COOPCLOSE
}

func (s CoopCloseMessage) Validate() error {
	panic("implement me")
}

func MarshalPeerswapMessage(msg PeerMessage) ([]byte, int, error) {
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return nil, 0, err
	}
	return msgBytes, int(msg.MessageType()), nil
}
