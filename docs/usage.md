# Usage guide

PeerSwap is a Peer To Peer atomic swap plugin for lightning nodes. It allows for channel rebalincing via atomic swaps with onchain coins. Supported blockchains:

- btc (bitcoin)
- l-btc (liquid)


## Notes on commands

every command can be run with c-lightning plugins interface or using pscli.

For the c-lightning plugin you need to prepend `lightning-cli peerswap-<command>`.

For the standalone daemon you would run `pscli <command>`

E.g. the `liquid-getaddress` command would look like this

```bash
lightning-cli peerswap-liquid-getaddress ## c-lightning plugin call
pscli liquid-getaddress ## standalone daemon call
```

In order to list all peerswap calls run
LND:

```pscli help```

c-lightningplugin:

```lightning-cli help | grep -A 1 peerswap```

## Liquid Usage

If you have set up your wallet with liquid swaps enabled you can swap with your peers using l-btc.

In order to swap you need a minimum balance of liquid bitcoin in order to pay for transaction fees.

The liquid wallet related commands are

```bash
liquid-getaddress ## generates a new liquid address
liquid-getbalance ## gets liquid bitcoin balance in sats
liquid-sendtoaddress ## sends lbtc sats to a provided address
```

The liquid wallet uses the elementsd integrated wallet

## Swaps

A swap is a atomic swap process between on-chain and lightning. A swap consists of two on-chain transaction and a lightning payment. The first transaction commits to the swap. Once confirmed the other party pays the lightning payment and spends the first transaction using the payment preimage.
There are two types of swap possible.

### Swap-Out

A swap out is when the initiator wants to pay a lightning payment in order to receive on-chain funds, in channel balancing terms receiving inbound liquidity. In order to swap out you need a minimum balance of liquid bitcoin in order to pay for transaction fees.

To swap out call

```bash
swap-out [amount in sats] [short channel id] [asset: btc or l-brc]
```


### Swap-In

A swap out is when the initiator wants to spend onchain bitcoin in order to receive lightning-funds, in channel balancing terms increasing outbound liquidity. In order to swap in you need to 

To swap in call

```bash
swap-in [amount in sats] [short channel id] [asset: btc or l-brc]
```

## Misc
`listpeers` - command that returns peers that support the peerswap protocol. It also gives statistics about received and sent swaps to a peer.

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

`listswaps [pretty bool (optional)]` - command that lists all swaps. If _pretty_ is set the output is in a human readable format

`listactiveswaps` - list all ongoing swaps, relevant for upgrading peerswap

`listswaprequests` - lists rejected swaps requested by peer nodes.

Example output:
```json
[
   {
      "node_id": "...",
      "requests": {
         "swap out": {
            "l-btc": {
               "total_amount_sat": 3600,
               "n_requests": 3
            }
         }
      }
   }
]
```

`getswap [swapid]` - command that returns the swap with _swapid_

`reloadpolicy` - updates the changes made to the policy file

`rejectswaps [bool]` reject incoming swaps 
