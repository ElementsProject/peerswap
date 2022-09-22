# Usage Guide

PeerSwap is a Peer To Peer atomic swap plugin for lightning nodes. It allows for channel rebalincing via atomic swaps with onchain coins. Supported blockchains:

- btc (bitcoin)
- lbtc (liquid)


## Notes on commands

every command can be run with core-lightning plugins interface or using pscli.

For the cln plugin you need to prepend `lightning-cli peerswap-<command>`.

For the standalone daemon you would run `pscli <command>`

E.g. the `lbtc-getaddress` command would look like this

```bash
lightning-cli peerswap-lbtc-getaddress ## cln plugin call
pscli lbtc-getaddress                  ## lnd peerswap call
```

In order to list all peerswap calls run
LND:

```pscli help```

core-lightning plugin:

```lightning-cli help | grep -A 1 peerswap```

## Liquid Usage

PeerSwap automatically enables a Liquid L-BTC wallet (asset type `lbtc`) if it detects elementsd in default paths or is otherwise configured to connect to elementsd RPC.

The liquid wallet related commands are

```bash
lbtc-getaddress ## generates a new lbtc address
lbtc-getbalance ## gets lbtc bitcoin balance in sats
lbtc-sendtoaddress ## sends lbtc sats to a provided address
```

The liquid wallet uses an elementsd integrated wallet named `peerswap`. It creates a new wallet of that name if it does not already exist. The default location on disk for this wallet is `~/.elements/liquidv1/wallets/peerswap/wallet.dat`

## Swaps

PeerSwap facilitates a trustless atomic swap between on-chain and lightning channel balance. Each atomic swap consists of two on-chain transactions and a lightning payment. The first onchain transaction commits to the swap then waits a minimum quantity of confirmations to guard against double-spending. Once confirmed the other party pays the lightning payment which reveals the preimage, thereby enabling the onchain commitment to be claimed and the atomic swap is complete.

There are two types of swaps.

### Swap-Out

A swap-out is when the initiator wants to pay a lightning payment in order to receive on-chain funds. From the perspective of channel balancing the initiator gains inbound liquidity.

For CLN:
```bash
swap-out [amount in sats] [short channel id] [asset: btc or lbtc]
```

For LND:
```bash
swapout --sat_amt [amount in sats] --channel-id [chan_id] --asset [btc or lbtc]
```

### Swap-In

A swap-in is when the initiator wants to spend onchain bitcoin in order to receive lightning funds. From the perspective of balancing terms they gain outbound liquidity.

For CLN:
```bash
swap-in [amount in sats] [short channel id] [asset: btc or lbtc]
```

For LND:
```bash
swapin --sat_amt [amount in sats] --channel_id [chan_id] --asset [btc or lbtc]
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

`listswaps [detailed bool (optional)]` - command that lists all swaps. If _detailed_ is set the output shows the swap data as it is saved in the database

`listactiveswaps` - list all ongoing swaps, relevant for upgrading peerswap

`listswaprequests` - lists rejected swaps requested by peer nodes.

Example output:
```json
[
   {
      "node_id": "...",
      "requests": {
         "swap out": {
            "lbtc": {
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

`addpeer [peer_pubkey]` - adds a peer to the allowlist file

`removepeer [peer_pubkey]` - remove a peer from the allowlist file

`allowswaprequests [bool]` - sets whether peerswap should allow new swap requests.
