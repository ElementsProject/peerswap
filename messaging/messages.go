package messaging

const (

	MESSAGETYPE_SWAPINREQUEST    = "a455"
	MESSAGETYPE_SWAPOUTREQUEST   = "a457"
	MESSAGETYPE_SWAPINAGREEMENT   = "a459"
	MESSAGETYPE_FEERESPONSE      = "a461"
	MESSAGETYPE_TXOPENEDRESPONSE = "a463"
	MESSAGETYPE_CANCELED         = "a465"
	MESSAGETYPE_CLAIMED          = "a467"
)

type ClaimType int

const (
	CLAIMTYPE_PREIMAGE = iota
	CLAIMTYPE_CLTV
)

	// SwapInRequest gets send when a peer wants to start a new swap.
type SwapInRequest struct {
	SwapId          string
	ChannelId       string
	Amount          uint64
}

func (s *SwapInRequest) MessageType() string {
	return MESSAGETYPE_SWAPINREQUEST
}

// SwapOutRequest gets send when a peer wants to start a new swap.
type SwapOutRequest struct {
	SwapId          string
	ChannelId       string
	Amount          uint64
	TakerPubkeyHash string
}

func (s *SwapOutRequest) MessageType() string {
	return MESSAGETYPE_SWAPOUTREQUEST
}

type FeeResponse struct {
	SwapId string
	Invoice string
}

func (s *FeeResponse) MessageType() string {
	return MESSAGETYPE_FEERESPONSE
}

type SwapInAgreementResponse struct {
	SwapId string
	TakerPubkeyHash string
}

func (s *SwapInAgreementResponse) MessageType() string {
	return MESSAGETYPE_SWAPINAGREEMENT
}


type TxOpenedResponse struct {
	SwapId string
	MakerPubkeyHash string
	Invoice string
	TxId string
	Cltv int64
}

func (t *TxOpenedResponse) MessageType() string {
	return MESSAGETYPE_TXOPENEDRESPONSE
}


type ClaimedMessage struct {
	SwapId    string
	ClaimType ClaimType
	ClaimTxId string
}

func (s *ClaimedMessage) MessageType() string {
	return MESSAGETYPE_CLAIMED
}

type CancelResponse struct {
	SwapId string
	Error  string
}

func (e *CancelResponse) MessageType() string {
	return MESSAGETYPE_CANCELED
}

