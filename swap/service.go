package swap

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/sputn1ck/peerswap/blockchain"
	"github.com/sputn1ck/peerswap/lightning"
	"github.com/sputn1ck/peerswap/messaging"
	"github.com/sputn1ck/peerswap/utils"
	"github.com/sputn1ck/peerswap/wallet"
	"github.com/vulpemventures/go-elements/elementsutil"
	"github.com/vulpemventures/go-elements/network"
	"log"
)

const (
	FIXED_FEE = 2000
	LOCKTIME  = 120
)

type TxBuilder interface {
}

type LightningClient interface {
	GetPayreq(amount uint64, preImage string, label string) (string, error)
	GetFeePayreq(amount uint64, label string) (string, error)
	DecodePayreq(payreq string) (*lightning.Invoice, error)
	PayInvoice(payreq string) (preimage string, err error)
	GetPreimage() (lightning.Preimage, error)
	GetNodeId() string
}

type Policy interface {
	GetMakerFee(swapValue uint64, swapFee uint64) (uint64, error)
	GetSwapInAgreement(swapValue uint64) (bool, error)
}

type SwapStore interface {
	Create(*Swap) error
	Update(*Swap) error
	DeleteById(string) error
	GetById(string) (*Swap, error)
	ListAll() ([]*Swap, error)
}

func (s *Service) getAsset() []byte {
	return append(
		[]byte{0x01},
		elementsutil.ReverseBytes(h2b(s.network.AssetID))...,
	)
}

type Service struct {
	store      SwapStore
	wallet     wallet.Wallet
	pc         messaging.PeerCommunicator
	blockchain blockchain.Blockchain
	lightning  LightningClient
	network    *network.Network
	txWatcher  *SwapWatcher
	policy Policy
	ctx context.Context
}

func NewService(ctx context.Context, store SwapStore, wallet wallet.Wallet, pc messaging.PeerCommunicator, blockchain blockchain.Blockchain, lightning LightningClient, network *network.Network) *Service {
	service := &Service{
		store:      store,
		wallet:     wallet,
		pc:         pc,
		blockchain: blockchain,
		lightning:  lightning,
		network:    network,
		ctx:        ctx}
	watchList := newTxWatcher(ctx, blockchain, service.preimageSwapCallback, service.timeLockSwapCallback)
	service.txWatcher = watchList
	return service
}

func (s *Service) ListSwaps() ([]*Swap, error) {
	return s.store.ListAll()
}

func (s *Service) StartSwapOut(peerNodeId string, channelId string, amount uint64) (*Swap, error)  {

	swap := NewSwap(SWAPTYPE_OUT, SWAPROLE_TAKER, amount, s.lightning.GetNodeId(), peerNodeId, channelId)
	err := s.store.Create(swap)
	if err != nil {
		return nil, err
	}
	pubkey := swap.GetPrivkey().PubKey()
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	request := &messaging.SwapOutRequest{
		SwapId:          swap.Id,
		ChannelId:       channelId,
		Amount:          amount,
		TakerPubkeyHash: swap.TakerPubkeyHash,
	}
	err = s.pc.SendMessage(peerNodeId, request)
	if err != nil {
		return nil, err
	}
	swap.State = SWAPSTATE_REQUEST_SENT
	err = s.store.Update(swap)
	if err != nil {
		return nil, err
	}
	return swap, nil
}
func (s *Service) StartSwapIn(peerNodeId string, channelId string, amount uint64) (*Swap, error) {
	swap := NewSwap(SWAPTYPE_IN, SWAPROLE_MAKER, amount, s.lightning.GetNodeId(), peerNodeId, channelId)
	err := s.store.Create(swap)
	if err != nil {
		return nil,err
	}
	request := &messaging.SwapInRequest{
		SwapId:          swap.Id,
		ChannelId:       channelId,
		Amount:          amount,
	}
	err = s.pc.SendMessage(peerNodeId, request)
	if err != nil {
		return nil, err
	}
	swap.State = SWAPSTATE_REQUEST_SENT
	err = s.store.Update(swap)
	if err != nil {
		return nil, err
	}
	return swap, nil
}

func (s *Service) OnSwapInRequest(senderNodeId string, request messaging.SwapInRequest) error {
	swap := NewSwapFromRequest(senderNodeId, request.SwapId, request.Amount, request.ChannelId, SWAPTYPE_IN)
	err := s.store.Create(swap)
	if err != nil {
		return err
	}

	pubkey := swap.GetPrivkey().PubKey()


	swap.Role = SWAPROLE_TAKER
	swap.TakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())

	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	agree, err := s.policy.GetSwapInAgreement(request.Amount)
	if err != nil {
		return err
	}
	var response messaging.PeerMessage
	if agree {
		response = &messaging.SwapInAgreementResponse{
			SwapId:          swap.Id,
			TakerPubkeyHash: hex.EncodeToString(pubkey.SerializeCompressed()),
		}
	} else {
		response = &messaging.CancelResponse{
			SwapId:          swap.Id,
			Error: "swap not possible",
		}
	}

	err = s.pc.SendMessage(swap.PeerNodeId, response)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) OnSwapOutRequest(senderNodeId string, request messaging.SwapOutRequest) error {
	ctx := context.Background()

	// todo create funded transaction here, if not enough funds we can cancel the swap directly
	swap := NewSwapFromRequest(senderNodeId, request.SwapId, request.Amount, request.ChannelId, SWAPTYPE_OUT)
	err := s.store.Create(swap)
	if err != nil {
		return err
	}

	pubkey := swap.GetPrivkey().PubKey()

	swap.Role = SWAPROLE_MAKER
	swap.TakerPubkeyHash = request.TakerPubkeyHash
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := s.lightning.GetPreimage()
	if err != nil {
		return err
	}
	pHash := preimage.Hash()
	log.Printf("maker preimage: %s ", preimage.String())
	payreq, err := s.lightning.GetPayreq((request.Amount+FIXED_FEE)*1000, preimage.String(), swap.Id)
	if err != nil {
		return err
	}

	swap.Payreq = payreq
	swap.PreImage = preimage.String()
	swap.PHash = pHash.String()
	swap.State = SWAPSTATE_OPENING_TX_PREPARED
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	err = s.CreateOpeningTransaction(ctx, swap)
	if err != nil {
		return err
	}

	fee, err := s.policy.GetMakerFee(request.Amount, swap.OpeningTxFee)
	if err != nil {
		return err
	}

	feeInvoice, err := s.lightning.GetFeePayreq(fee, swap.Id + "_fee")
	if err != nil {
		return err
	}

	response := &messaging.FeeResponse{
		SwapId:          swap.Id,
		Invoice:         feeInvoice,
	}
	err = s.pc.SendMessage(swap.PeerNodeId, response)
	if err != nil {
		return err
	}

	return nil
}

func (s *Service) OnFeeInvoicePaid(swapId string) error {
	swap, err := s.store.GetById(swapId)
	if err != nil {
		return err
	}

	txId, err := s.wallet.FinalizeAndBroadcastFundedTransaction(swap.OpeningTxUnpreparedHex)
	swap.OpeningTxId = txId
	swap.State = SWAPSTATE_OPENING_TX_BROADCASTED
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	s.txWatcher.AddSwap(swap)
	response := &messaging.TxOpenedResponse{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.Payreq,
		TxId: swap.OpeningTxId,
	}
	err = s.pc.SendMessage(swap.PeerNodeId, response)
	if err != nil {
		return err
	}

	return nil
}

// CreateOpeningTransaction creates the opening Transaction,
// the two peers are the taker(pays the invoice) and the maker (provides onchain liquidity)
func (s *Service) CreateOpeningTransaction(ctx context.Context, swap *Swap) error {
	// Create the opening transaction
	blockHeight, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return err
	}
	spendingBlocktimeHeight := int64(blockHeight + LOCKTIME)
	swap.Cltv = spendingBlocktimeHeight
	redeemScript, err := s.getSwapScript(swap)
	if err != nil {
		return err
	}

	openingTx, err := utils.CreateOpeningTransaction(redeemScript, s.getAsset(), swap.Amount)
	if err != nil {
		return err
	}


	txHex, fee, err := s.wallet.CreateFundedTransaction(openingTx)
	if err != nil {
		return err
	}
	vout, err := utils.VoutFromTxHex(txHex, redeemScript)
	if err != nil {
		return err
	}

	swap.OpeningTxUnpreparedHex = txHex
	swap.OpeningTxFee = fee
	swap.OpeningTxVout = vout
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	return nil
}
func (s *Service) OnSwapInAgreement(senderNodeId string, response messaging.SwapInAgreementResponse) error {
	swap, err := s.store.GetById(response.SwapId)
	if err != nil {
		return err
	}
	if swap.PeerNodeId != senderNodeId {
		return errors.New("peer has changed, aborting")
	}

	pubkey := swap.GetPrivkey().PubKey()

	swap.TakerPubkeyHash = response.TakerPubkeyHash
	swap.MakerPubkeyHash = hex.EncodeToString(pubkey.SerializeCompressed())
	// Generate Preimage
	preimage, err := s.lightning.GetPreimage()
	if err != nil {
		return err
	}
	pHash := preimage.Hash()
	log.Printf("maker preimage: %s ", preimage.String())
	payreq, err := s.lightning.GetPayreq((swap.Amount+FIXED_FEE)*1000, preimage.String(), swap.Id)
	if err != nil {
		return err
	}

	swap.Payreq = payreq
	swap.PreImage = preimage.String()
	swap.PHash = pHash.String()
	swap.State = SWAPSTATE_OPENING_TX_PREPARED
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	err = s.CreateOpeningTransaction(context.Background(), swap)
	if err != nil {
		return err
	}
	return s.BroadCastAndNotifyPeer(swap)
}

func (s *Service) BroadCastAndNotifyPeer(swap *Swap) error {
	txId, err := s.wallet.FinalizeAndBroadcastFundedTransaction(swap.OpeningTxUnpreparedHex)
	swap.OpeningTxId = txId
	swap.State = SWAPSTATE_OPENING_TX_BROADCASTED
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	s.txWatcher.AddSwap(swap)
	response := &messaging.TxOpenedResponse{
		SwapId:          swap.Id,
		MakerPubkeyHash: swap.MakerPubkeyHash,
		Invoice:         swap.Payreq,
		TxId: swap.OpeningTxId,
	}
	err = s.pc.SendMessage(swap.PeerNodeId, response)
	if err != nil {
		return err
	}
	return nil
}
func (s *Service) OnMakerResponse(senderNodeId string, request messaging.TxOpenedResponse) error {
	swap, err := s.store.GetById(request.SwapId)
	if err != nil {
		return err
	}
	if swap.PeerNodeId != senderNodeId {
		return errors.New("peer has changed, aborting")
	}
	swap.State = SWAPSTATE_WAITING_FOR_TX
	swap.MakerPubkeyHash = request.MakerPubkeyHash
	swap.Payreq = request.Invoice
	swap.OpeningTxId = request.TxId
	swap.OpeningTxUnpreparedHex = request.TxHex
	swap.OpeningTxVout = request.Vout
	swap.Cltv = request.Cltv

	invoice, err := s.lightning.DecodePayreq(swap.Payreq)
	if err != nil {
		return err
	}

	swap.PHash = invoice.PHash

	if invoice.Amount > (swap.Amount+FIXED_FEE)*1000 {
		return errors.New(fmt.Sprintf("invoice amount is to high, got: %v, expected %v", swap.Amount+FIXED_FEE, invoice.Amount))
	}

	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	s.txWatcher.AddSwap(swap)
	return nil
}

func (s *Service) OnClaimedResponse(senderNodeId string, request ClaimedMessage) error {
	swap, err := s.store.GetById(request.SwapId)
	if err != nil {
		return err
	}
	swap.State = SwapState(int(SWAPSTATE_CLAIMED_PREIMAGE) + int(request.ClaimType))
	swap.ClaimTxId = request.ClaimTxId
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) OnErrorMessage(peerId string, errorMessage CancelResponse) error {
	swap, err := s.store.GetById(errorMessage.SwapId)
	if err != nil {
		return err
	}
	swap.State = SWAPSTATE_CANCELED
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) StartWatchingTxs() error {
	swaps, err := s.store.ListAll()
	if err != nil {
		return err
	}
	err = s.txWatcher.StartWatchingTxs(swaps)
	if err != nil {
		return err
	}
	return nil
}
func (s *Service) ClaimTxWithPreimage(ctx context.Context, swap *Swap, openingTxHex string) error {

	if swap.PreImage == "" {
		preimageString, err := s.lightning.PayInvoice(swap.Payreq)
		if err != nil {
			return err
		}
		swap.PreImage = preimageString
		err = s.store.Update(swap)
		if err != nil {
			return err
		}
	}
	preimage, err := lightning.MakePreimageFromStr(swap.PreImage)
	if err != nil {
		return err
	}
	redeemScript, err := s.getSwapScript(swap)
	if err != nil {
		return err
	}

	blockheight, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return err
	}

	address, err := s.wallet.GetAddress()
	if err != nil {
		return err
	}

	outputScript, err := utils.Blech32ToScript(address, s.network)
	if err != nil {
		return err
	}

	claimTxHex, err := utils.CreatePreimageSpendingTransaction(&utils.SpendingParams{
		Signer:       swap.GetPrivkey(),
		OpeningTxHex: openingTxHex,
		SwapAmount:   swap.Amount,
		FeeAmount:    FIXED_FEE,
		CurrentBlock: blockheight,
		Asset:        s.getAsset(),
		OutputScript: outputScript,
		RedeemScript: redeemScript,
	}, preimage[:])

	claimId, err := s.blockchain.SendRawTx(claimTxHex)
	if err != nil {
		return err
	}
	swap.ClaimTxId = claimId
	swap.State = SWAPSTATE_CLAIMED_PREIMAGE

	log.Printf("taker claimid %s", claimId)
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) ClaimTxWithCltv(ctx context.Context, swap *Swap, openingTxHex string) error {
	redeemScript, err := s.getSwapScript(swap)
	if err != nil {
		return err
	}

	blockheight, err := s.blockchain.GetBlockHeight()
	if err != nil {
		return err
	}

	address, err := s.wallet.GetAddress()
	if err != nil {
		return err
	}

	outputScript, err := utils.Blech32ToScript(address, s.network)
	if err != nil {
		return err
	}

	claimTxHex, err := utils.CreateCltvSpendingTransaction(&utils.SpendingParams{
		Signer:       swap.GetPrivkey(),
		OpeningTxHex: openingTxHex,
		SwapAmount:   swap.Amount,
		FeeAmount:    FIXED_FEE,
		CurrentBlock: blockheight,
		Asset:        s.getAsset(),
		OutputScript: outputScript,
		RedeemScript: redeemScript,
	})

	claimId, err := s.blockchain.SendRawTx(claimTxHex)
	if err != nil {
		log.Printf("error claiming tx %v", err)
		return err
	}
	swap.ClaimTxId = claimId
	swap.State = SWAPSTATE_CLAIMED_TIMELOCK

	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) preimageSwapCallback(swapId string) error {
	swap, err := s.store.GetById(swapId)
	if err != nil {
		return err
	}
	txHex, err := s.blockchain.GetRawtransaction(swap.OpeningTxId)
	if err != nil {
		return err
	}
	err = s.ClaimTxWithPreimage(s.ctx, swap, txHex)
	if err != nil {
		return err
	}
	swap.State = SWAPSTATE_CLAIMED_PREIMAGE
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	claimedMessage := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_PREIMAGE,
		ClaimTxId: swap.ClaimTxId,
	}
	err = s.pc.SendMessage(swap.PeerNodeId, claimedMessage)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) timeLockSwapCallback(swapId string) error {
	swap, err := s.store.GetById(swapId)
	if err != nil {
		return err
	}
	txHex, err := s.blockchain.GetRawtransaction(swap.OpeningTxId)
	if err != nil {
		return err
	}
	err = s.ClaimTxWithCltv(s.ctx, swap, txHex)
	if err != nil {
		return err
	}
	swap.State = SWAPSTATE_CLAIMED_TIMELOCK
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	claimedMessage := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_CLTV,
		ClaimTxId: swap.ClaimTxId,
	}
	err = s.pc.SendMessage(swap.PeerNodeId, claimedMessage)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) timelockCallback(swapId string) error {
	swap, err := s.store.GetById(swapId)
	if err != nil {
		return err
	}
	txHex, err := s.blockchain.GetRawtransaction(swap.OpeningTxId)
	if err != nil {
		return err
	}
	err = s.ClaimTxWithPreimage(s.ctx, swap, txHex)
	if err != nil {
		return err
	}
	swap.State = SWAPSTATE_CLAIMED_TIMELOCK
	err = s.store.Update(swap)
	if err != nil {
		return err
	}
	claimedMessage := &ClaimedMessage{
		SwapId:    swap.Id,
		ClaimType: CLAIMTYPE_CLTV,
		ClaimTxId: swap.ClaimTxId,
	}
	err = s.pc.SendMessage(swap.PeerNodeId, claimedMessage)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) getSwapScript(swap *Swap) ([]byte, error) {
	// check script
	takerPubkeyHashBytes, err := hex.DecodeString(swap.TakerPubkeyHash)
	if err != nil {
		return nil, err
	}
	makerPubkeyHashBytes, err := hex.DecodeString(swap.MakerPubkeyHash)
	if err != nil {
		return nil, err
	}
	pHashBytes, err := hex.DecodeString(swap.PHash)
	if err != nil {
		return nil, err
	}
	script, err := utils.GetOpeningTxScript(takerPubkeyHashBytes, makerPubkeyHashBytes, pHashBytes, swap.Cltv)
	if err != nil {
		return nil, err
	}
	log.Printf("\n scriptvals: %s %s %s %v \nscripthex: %s", swap.TakerPubkeyHash, swap.MakerPubkeyHash, swap.PHash, swap.Cltv, hex.EncodeToString(script))
	return script, nil
}
func b2h(buf []byte) string {
	return hex.EncodeToString(buf)
}
func h2b(str string) []byte {
	buf, _ := hex.DecodeString(str)
	return buf
}
