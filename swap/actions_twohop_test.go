package swap

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/elementsproject/peerswap/policy"
	"github.com/elementsproject/peerswap/premium"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
)

func newTestPremiumSetting(t *testing.T) *premium.Setting {
	t.Helper()

	dir := t.TempDir()
	db, err := bbolt.Open(path.Join(dir, "premium-db"), os.ModePerm, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	premiumSetting, err := premium.NewSetting(db)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t,
		premiumSetting.SetDefaultRate(ctx,
			lo.Must(premium.NewPremiumRate(premium.BTC, premium.SwapIn, premium.NewPPM(10000)))))
	require.NoError(t,
		premiumSetting.SetDefaultRate(ctx,
			lo.Must(premium.NewPremiumRate(premium.BTC, premium.SwapOut, premium.NewPPM(10000)))))
	require.NoError(t,
		premiumSetting.SetDefaultRate(ctx,
			lo.Must(premium.NewPremiumRate(premium.LBTC, premium.SwapIn, premium.NewPPM(10000)))))
	require.NoError(t,
		premiumSetting.SetDefaultRate(ctx,
			lo.Must(premium.NewPremiumRate(premium.LBTC, premium.SwapOut, premium.NewPPM(10000)))))

	return premiumSetting
}

func TestCreateSwapOutFromRequestAction_SetsTwoHopIncomingScid(t *testing.T) {
	t.Parallel()

	swapID := NewSwapId()
	localScid := "22x2x2"

	swapData := &SwapData{
		PeerNodeId:   getRandom33ByteHexString(),
		PrivkeyBytes: getRandomPrivkey().Serialize(),
		LocalScid:    localScid,
		SwapOutRequest: &SwapOutRequestMessage{
			ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
			SwapId:          swapID,
			Asset:           "",
			Network:         "mainnet",
			Scid:            "100x2x3",
			Amount:          100000,
			Pubkey:          getRandom33ByteHexString(),
			PremiumLimit:    999999999,
			TwoHop: &TwoHop{
				IntermediaryPubkey: getRandom33ByteHexString(),
			},
		},
	}

	chain := &dummyChain{returnGetCSVHeight: 1008}
	chain.SetBalance(10_000_000)
	lc := &dummyLightningClient{preimage: ""}

	services := &SwapServices{
		lightning:        lc,
		bitcoinTxWatcher: chain,
		bitcoinWallet:    chain,
		bitcoinValidator: chain,
		bitcoinEnabled:   true,
		liquidTxWatcher:  chain,
		liquidWallet:     chain,
		liquidValidator:  chain,
		liquidEnabled:    true,
		messenger:        &dummyMessenger{},
		messengerManager: &MessengerManagerStub{},
		policy: &dummyPolicy{
			getMinSwapAmountMsatReturn: policy.DefaultPolicy().MinSwapAmountMsat,
			newSwapsAllowedReturn:      policy.DefaultPolicy().AllowNewSwaps,
		},
		toService: &timeOutDummy{},
		ps:        newTestPremiumSetting(t),
	}

	a := &CreateSwapOutFromRequestAction{}
	got := a.Execute(services, swapData)
	assert.Equal(t, Event_ActionSucceeded, got)
	require.NotNil(t, swapData.SwapOutAgreement)
	require.NotNil(t, swapData.SwapOutAgreement.TwoHop)
	assert.Equal(t, localScid, swapData.SwapOutAgreement.TwoHop.IncomingScid)
}

func TestPayFeeInvoiceAction_Uses2HopPaymentWhenRequested(t *testing.T) {
	t.Parallel()

	swapID := NewSwapId()
	intermediary := getRandom33ByteHexString()
	outgoingScid := "111x1x1"
	incomingScid := "222x2x2"

	swapData := &SwapData{
		LocalScid: outgoingScid,
		SwapOutRequest: &SwapOutRequestMessage{
			ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
			SwapId:          swapID,
			Asset:           "",
			Network:         "mainnet",
			Scid:            outgoingScid,
			Amount:          100000,
			Pubkey:          getRandom33ByteHexString(),
			PremiumLimit:    999999999,
			TwoHop: &TwoHop{
				IntermediaryPubkey: intermediary,
			},
		},
		SwapOutAgreement: &SwapOutAgreementMessage{
			ProtocolVersion: PEERSWAP_PROTOCOL_VERSION,
			SwapId:          swapID,
			Pubkey:          getRandom33ByteHexString(),
			Payreq:          "fee",
			Premium:         0,
			TwoHop: &TwoHop{
				IncomingScid: incomingScid,
			},
		},
	}

	chain := &dummyChain{returnGetCSVHeight: 1008}
	lc := &dummyLightningClient{preimage: ""}

	services := &SwapServices{
		lightning:        lc,
		bitcoinTxWatcher: chain,
		bitcoinWallet:    chain,
		bitcoinValidator: chain,
		bitcoinEnabled:   true,
	}

	a := &PayFeeInvoiceAction{}
	got := a.Execute(services, swapData)
	assert.Equal(t, Event_ActionSucceeded, got)
	assert.Equal(t, 1, lc.payInvoiceVia2HopRouteCalled)
	assert.Equal(t, "fee", lc.payInvoiceVia2HopRouteParams.payreq)
	assert.Equal(t, outgoingScid, lc.payInvoiceVia2HopRouteParams.outgoingScid)
	assert.Equal(t, incomingScid, lc.payInvoiceVia2HopRouteParams.incomingScid)
	assert.Equal(t, intermediary, lc.payInvoiceVia2HopRouteParams.intermediaryPubkey)
	assert.NotEmpty(t, swapData.FeePreimage)
}
