# PeerSwap Protocol

- [PeerSwap Protocol](#peerswap-protocol)
  - [Diagrams](#diagrams)
    - [Swap out](#swap-out)
    - [Swap in](#swap-in)
  - [Messages](#messages)
    - [Swap out request](#swap-out-request)
    - [Swap in request](#swap-in-request)
    - [Swap out agreement response](#swap-out-agreement-response)
    - [Swap in agreement response](#swap-in-agreement-response)
    - [Tx opened response](#tx-opened-response)
    - [Cancel message](#cancel-message)
  - [Notes](#notes)
    - [Premiums](#premiums)
    - [Opening Transaction](#opening-transaction)
    - [Misc](#misc)
  
## Diagrams

### Swap out

![swap out](img/swap-out-sequence.png)

### Swap in

![swap in](img/swap-in-sequence.png)

## Messages

### Swap out request

```go
{
  SwapId: string // Random string that is shared between peers
  ProtocolVersion: uint64
  Asset: string // btc or l-btc
  ChannelId: string // chhannelId of rebalanced channel
  Amount: uint64  // amount to be swapped (in msats)
  PubkeyHash: string  // Taker PubkeyHash, for creating/verifying the bitcoin script
}
```

### Swap in request

```go
{
  SwapId: string
  ProtocolVersion: uint64
  Asset: string // btc or l-btc
  ChannelId: string // chhannelId of rebalanced channel
  Amount: uint64  // amount to be swapped (in msats)
  PubkeyHash: string  // Maker PubkeyHash, for creating/verifying the bitcoin script
}
```

### Swap out agreement response

```go
{
  SwapId: string 
  Invoice: string  // Bolt11 string of fee invoice
  Premium: uint64 // Premium that bob wants in order to accept the swap
}
```

### Swap in agreement response

```go
{
  SwapId: string
  PubkeyHash: string // Taker PubkeyHash, for creating/verifying the bitcoin script
  Premium: uint64 // Premium that bob wants in order to accept the swap
}
```

### Tx opened response

```go
{
  SwapId: string 
  PubkeyHash: string // Maker pubkey hash, for verifying the bitcoin script
  Payreq: string // Invoice that claims the transaction
  TxId: string // TxId of broadcasted commitment transaction
  MakerRefundAddr: string // Onchain address for coop close
  RefundFee: uint64 // Refund fee for coop close
}
```
### CoopClose message
```go
{
    SwapId:             string
    TakerRefundSigHash: string // Sighash of refund transaction, that was ubilt using MakerRefundAddr and RefundFee 
}
```
### Cancel message

```go
{
  SwapId: string
  Msg: string // Some information why the swap was canceled
}
```

## Notes


### Opening Transaction

```bash
#Miniscript:
or(and(pk(A),sha256(H)),and(pk(B),older(N)))

# Bitcoin/Elements script
<A> OP_CHECKSIG OP_NOTIF
  <B> OP_CHECKSIGVERIFY <N> OP_CHECKSEQUENCEVERIFY
OP_ELSE
  OP_SIZE <20> OP_EQUALVERIFY OP_SHA256 <H> OP_EQUAL
OP_ENDIF

```

### Misc

- Taker is the one claiming the commitment transaction
- Maker the one creating it and providing lbtc liquidity
- Swap In means paying lbtc to get sats
- Swap Out means paying an invoice to get lbtc
