`swap_in_request`
```
{
	ProtocolVersion uint64
    SwapId          string
	Asset           string
	ChannelId       string
	Amount          uint64
}
```

`swap_in_agreement`
```
{
    ProtocolVersion uint64
    SwapId          string
	TakerPubkeyHash string
}
```

`swap_out_request`
```
{
	ProtocolVersion uint64
    SwapId          string
	Asset           string
	ChannelId       string
	Amount          uint64
	TakerPubkeyHash string
}
```

`swap_out_agreement`
```
{
	ProtocolVersion uint64
    SwapId  		string
	FeeInvoice 		string
}
```

`opening_tx_broadcasted`
```
{
    SwapId          string
	MakerPubkeyHash string
	RefundAddr      string
	RefundFee       uint64
	TxHex           string
	Invoice         string
}
```

`coop_close`
```
{
    SwapId             string
	TakerRefundSigHash string
}
```

`cancel`
```
{
    SwapId string
	Error  string
}
```