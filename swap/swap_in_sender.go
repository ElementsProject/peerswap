package swap


const (
	State_SwapInSender_Init              StateType = "State_SwapInSender_Init"
	State_SwapInSender_Created           StateType = "State_SwapInSender_Created"
	State_SwapInSender_SwapInRequestSent StateType = "State_SwapInSender_SwapInRequestSent"
	State_SwapInSender_AgreementReceived StateType = "State_SwapInSender_AgreementReceived"
	State_SwapInSender_TxBroadcasted     StateType = "State_SwapInSender_TxBroadcasted"
	State_SwapInSender_TxMsgSent         StateType = "State_SwapInSender_TxMsgSent"
	State_SwapInSender_ClaimInvPaid      StateType = "State_SwapInSender_ClaimInvPaid"
	State_SwapInSender_CltvPassed StateType = "State_SwapInSender_CltvPassed"
	State_SwapInSender_ClaimedPreimage   StateType = "State_SwapInSender_ClaimedPreimage"
	State_SwapInSender_ClaimedCltv       StateType = "State_SwapInSender_ClaimedCltv"
	
	State_SwapCanceled StateType = "State_SwapCanceled"
	State_SendCancelThenWaitCltv StateType = "State_SendCancelThenWaitCltv"
	State_WaitCltv StateType = "State_WaitCltv"

	Event_SwapInSender_OnSwapInRequested EventType = "Event_SwapInSender_OnSwapInRequested"
	Event_SwapInSender_OnSwapInCreated EventType = "Event_SwapInSender_OnSwapInCreated"
	Event_SwapInSender_OnSwapInRequestSent EventType = "Event_SwapInSender_OnSwapInRequestSent"
	Event_SwapInSender_OnAgreementReceived EventType = "Event_SwapInSender_OnAgreementReceived"
	Event_SwapInSender_OnTxBroadcasted EventType = "Event_SwapInSender_OnTxBroadcasted"
	Event_SwapInSender_OnTxMsgSent EventType = "Event_SwapInSender_OnTxMsgSent"
	Event_SwapInSender_OnClaimInvPaid EventType = "Event_SwapInSender_OnClaimInvPaid"
	Event_SwapInSender_OnCltvPassed EventType = "Event_SwapInSender_OnCltvPassed"
	Event_SwapInSender_OnClaimTxPreimage EventType = "Event_SwapInSender_OnClaimTxPreimage"
	Event_SwapInSender_OnClaimTxCltv EventType = "Event_SwapInSender_OnClaimTxCltv"
	
	Event_ActionFailed EventType = "Event_ActionFailed"

)

// SwapInSenderInitAction creates the swap strcut
type SwapInSenderInitAction struct {}

func (s *SwapInSenderInitAction) Execute(services *SwapServices, swap *Swap) EventType {
	panic("implement me")
}

// SwapInSenderCreatedAction sends the request to the swap peer
type SwapInSenderCreatedAction struct {}

func (s *SwapInSenderCreatedAction) Execute(services *SwapServices, swap *Swap) EventType {
	panic("implement me")
}

// SwapInSenderAgreementReceivedAction creates and broadcasts the redeem transaction
type SwapInSenderAgreementReceivedAction struct{}

func (s *SwapInSenderAgreementReceivedAction) Execute(services *SwapServices, swap *Swap) EventType {
	panic("implement me")
}

// SwapInSenderTxBroadcastedAction sends the claim tx information to the swap peer
type SwapInSenderTxBroadcastedAction struct{}

func (s *SwapInSenderTxBroadcastedAction) Execute(services *SwapServices, swap *Swap) EventType {
	panic("implement me")
}

// SwapInSenderCltvPassedAction claims the claim tx and sends the claim msg to the swap peer
type SwapInSenderCltvPassedAction struct{}

func (s *SwapInSenderCltvPassedAction) Execute(services *SwapServices, swap *Swap) EventType {
	panic("implement me")
}


func getSwapInSenderStates() States {
	return States{
		Default: State{
			Events: Events{
				Event_SwapInSender_OnSwapInRequested: State_SwapInSender_Init,
			},
		},
		State_SwapInSender_Init: {
			Action: &SwapInSenderInitAction{},
			Events: Events{
				Event_SwapInSender_OnSwapInCreated: State_SwapInSender_Created,
				Event_ActionFailed: State_SwapCanceled,
			},
		},
		State_SwapInSender_Created: {
			Action: &SwapInSenderCreatedAction{},
			Events: Events{
				Event_SwapInSender_OnSwapInRequestSent: State_SwapInSender_SwapInRequestSent,
				Event_ActionFailed: State_SwapCanceled,
			},
		},
		State_SwapInSender_SwapInRequestSent: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapInSender_OnAgreementReceived: State_SwapInSender_AgreementReceived,
				Event_OnCancelReceived: State_SwapOut_Canceled,
			},
		},
		State_SwapInSender_AgreementReceived : {
			Action: &SwapInSenderAgreementReceivedAction{},
			Events: Events{
				Event_SwapInSender_OnTxBroadcasted: State_SwapInSender_TxBroadcasted,
				Event_ActionFailed: State_SendCancel,
			},
		},
		State_SwapInSender_TxBroadcasted: {
			Action: &SwapInSenderTxBroadcastedAction{},
			Events: Events{
				Event_SwapInSender_OnTxMsgSent: State_SwapInSender_TxMsgSent,
				Event_SwapInSender_OnCltvPassed: State_SwapInSender_CltvPassed,
				Event_ActionFailed: State_SendCancelThenWaitCltv,
			},
		},
		State_SwapInSender_TxMsgSent: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapInSender_OnClaimInvPaid: State_SwapInSender_ClaimInvPaid,
				Event_SwapInSender_OnCltvPassed: State_SwapInSender_CltvPassed,
				Event_OnCancelReceived: State_WaitCltv,
			},
		},
		State_SwapInSender_ClaimInvPaid: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapInSender_OnClaimTxPreimage: State_SwapInSender_ClaimedPreimage,
			},
		},
		State_SwapInSender_ClaimedPreimage: {
			Action: &NoOpAction{},
		},
		State_SwapInSender_CltvPassed: {
			Action: &SwapInSenderCltvPassedAction{},
			Events: Events{
				Event_SwapInSender_OnClaimTxCltv: State_SwapInSender_ClaimedCltv,
			},
		},
		State_SendCancelThenWaitCltv: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_Action_Success: State_WaitCltv,
			},
		},
		State_WaitCltv: {
			Action: &NoOpAction{},
			Events: Events{
				Event_SwapInSender_OnCltvPassed: State_SwapInSender_CltvPassed,
			},
		},
		State_SwapInSender_ClaimedCltv: {
			Action: &NoOpAction{},
		},

		State_SendCancel: {
			Action: &SendCancelAction{},
			Events: Events{
				Event_Action_Success: State_SwapCanceled,
			},
		},
		State_SwapCanceled: {
			Action: &NoOpAction{},
		},
	}
}