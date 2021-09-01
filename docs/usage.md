# Usage guide

PeerSwap is a Peer To Peer atomic swap plugin for lightning nodes.

Currently only swapping with liquid bitcoin is supported.

## Setup

In order to use peerswap start `lightningd` with the following options, replacing as necessary
```
lightningd \ 
 --peerswap-liquid-rpchost=http://localhost \
 --peerswap-liquid-rpcport=7041 \
 --peerswap-liquid-rpcuser=admin1 \
 --peerswap-liquid-rpcpassword=123 \
 --peerswap-liquid-network=regtest \
 --peerswap-liquid-rpcwallet=swap1
```

## Liquid Wallet

In order to swap you need a minimum balance of liquid bitcoin in order to pay for transaction fees.

The liquid wallet related commands are

```bash
peerswap-liquid-getaddress ## generates a new liquid address
peerswap-liquid-getbalance ## gets liquid bitcoin balance in sats
peerswap-liquid-sendtoaddress ## sends lbtc sats to a provided address
```

The liquid wallet uses the elementsd integrated wallet

## Swaps

A swap is a atomic swap process between on-chain and lightning. A swap consists of two on-chain transaction and a lightning payment. The first transaction commits to the swap. Once confirmed the other party pays the lightning payment and spends the first transaction using the payment preimage.
There are two types of swap possible.

### Swap-Out

A swap out is when the initiator wants to pay a lightning payment in order to receive on-chain funds, in channel balancing terms receiving inbound liquidity. In order to swap out you need a minimum balance of liquid bitcoin in order to pay for transaction fees.

To swap out call

```bash
peerswap-swap-out [amount in sats] [short channel id] [asset: btc or l-brc]
```


### Swap-In

A swap out is when the initiator wants to spend onchain bitcoin in order to receive lightning-funds, in channel balancing terms increasing outbound liquidity. In order to swap in you need to 

To swap in call

```bash
peerswap-swap-in [amount in sats] [short channel id] [asset: btc or l-brc]
```

## Misc
`peerswap-listpeers` - command that returns peers that support the peerswap protocol. It also gives statistics about received and sent swaps to a peer.

Example output:
```bash
[
   {
      "nodeid": "...",
      "channels": [
         {
            "short_channel_id": "...",
            "local_balance": 7397932,
            "remote_balance": 2602068,
            "balance": 0.7397932
         }
      ],
      "sent": {
         "total_swaps_out": 2,
         "total_swaps_in": 1,
         "total_sats_swapped_out": 5300000,
         "total_sats_swapped_in": 302938
      },
      "received": {
         "total_swaps_out": 1,
         "total_swaps_in": 0,
         "total_sats_swapped_out": 2400000,
         "total_sats_swapped_in": 0
      },
      "total_fee_paid": 6082
   }
]
```

`peerswap-listnodes` - command that returns nodes that support the peerswap plugin.

`peerswap-listswaps [pretty bool (optional)]` - command that lists all swaps. If _pretty_ is set the output is in a human readable format

`peetswap-getswap [swapid]` - command that returns the swap with _swapid_
