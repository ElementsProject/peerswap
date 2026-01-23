# Usage Guide

PeerSwap is a P2P atomic swap plugin for Lightning nodes. It allows for channel rebalancing via atomic swaps with onchain coins. Supported blockchains:

- Bitcoin
- [Liquid Network](https://docs.liquid.net/docs)


## Notes on commands

Every command can be run with CLN's plugins interface or using pscli.

For the CLN plugin you need to prepend `lightning-cli peerswap-<command>`.

For the LND standalone daemon you would run `pscli <command>`

E.g. the `lbtc-getaddress` command would look like this:

```bash
lightning-cli peerswap-lbtc-getaddress ## CLN plugin call
pscli lbtc-getaddress                  ## LND peerswap call
```

In order to list all PeerSwap calls run
CLN plugin:

`lightning-cli help | grep -A 1 peerswap`

LND:

`pscli help`


## Liquid Usage

PeerSwap automatically enables a Liquid L-BTC wallet (asset type `lbtc`) if it detects elementsd in default paths or is otherwise configured to connect to elementsd RPC.

The Liquid wallet related commands are:

For CLN:
Generate a new L-BTC address
`lightning-cli peerswap-lbtc-getaddress`

Get the L-BTC balance in sats
`lightning-cli peerswap-lbtc-getbalance`

Send L-BTC to a provided Liquid / L-BTC address
`lightning-cli peerswap-lbtc-sendtoaddress [address] [amount_sat]` 


For LND:
Generate a new L-BTC address
`pscli lbtc-getaddress`

Gets L-BTC balance in sats
`pscli lbtc-getbalance`

Send L-BTC to a provided Liquid / L-BTC address
`pscli lbtc-sendtoaddress --sat_amt [sat_amt] --address [address]`


The Liquid wallet uses an elementsd integrated wallet named `peerswap`. It creates a new wallet of that name if it does not already exist. The default location on disk for this wallet is `~/.elements/liquidv1/wallets/peerswap/wallet.dat`

## Swaps

PeerSwap facilitates a trustless atomic swap between on-chain and Lightning channel balance. Each atomic swap consists of two on-chain transactions and a Lightning payment. The first onchain transaction commits to the swap then waits a minimum quantity of confirmations to guard against double-spending. Once confirmed the other party pays the Lightning payment which reveals the preimage, thereby enabling the onchain commitment to be claimed and the atomic swap is complete.

There are two types of swaps.

### Swap-Out

A swap-out is when the initiator wants to pay a lightning payment in order to receive on-chain funds. From the perspective of channel balancing the initiator gains inbound liquidity.

For CLN:
```bash
lightning-cli peerswap-swap-out [short channel id] [amount in sats] [asset: btc or lbtc] [premium limit in ppm]
```

For LND:
```bash
pscli swapout --channel_id [chan_id] --sat_amt [amount in sats] --asset [btc or lbtc] --premium_limit_rate_ppm [premium limit in ppm]
```

### Swap-In

A swap-in is when the initiator wants to spend onchain bitcoin in order to receive lightning funds. From the perspective of balancing terms they gain outbound liquidity.

For CLN:
```bash
lightning-cli peerswap-swap-in [short channel id] [amount in sats] [asset: btc or lbtc] [premium limit in ppm]
```

For LND:
```bash
pscli swapin --channel_id [chan_id] --sat_amt [amount in sats] --asset [btc or lbtc] --premium_limit_rate_ppm [premium limit in ppm]
```

### 2-hop swaps (u→m→v)

PeerSwap can execute a swap even if the swap endpoints (`u` and `v`) do not share a direct channel, as long as there is exactly one intermediary node `m` with public channels `u–m` and `m–v`.

Requirements (high-level):

- `u` has a channel to the intermediary `m` (this is the channel you pass to the command).
- `m` has a public channel to `v` (for routing).
- `u` and `v` are connected as peers (TCP / Lightning peer connection), so they can exchange PeerSwap custom messages.
- `v` runs PeerSwap (the intermediary `m` does not need to).

If an intermediary `m` also runs PeerSwap, it may advertise `channel_adjacency` in `listpeers`, which can be used as a hint for picking a reachable `v` behind `m`.

To start a 2-hop swap, pass the channel to `m` as usual, and additionally set `peer_pubkey` to the *swap negotiation peer* `v`.

For CLN (named parameters recommended for optional fields):
```bash
lightning-cli peerswap-swap-out short_channel_id=<u-m scid> sat_amt=<sats> asset=btc premium_rate_limit_ppm=<ppm> peer_pubkey=<v pubkey>
lightning-cli peerswap-swap-in  short_channel_id=<u-m scid> sat_amt=<sats> asset=btc premium_rate_limit_ppm=<ppm> peer_pubkey=<v pubkey>
```

For LND:
```bash
pscli swapout --channel_id <u-m chan_id> --sat_amt <sats> --asset btc --premium_limit_rate_ppm <ppm> --peer_pubkey <v pubkey>
pscli swapin  --channel_id <u-m chan_id> --sat_amt <sats> --asset btc --premium_limit_rate_ppm <ppm> --peer_pubkey <v pubkey>
```

## Premium

The premium rate is the rate applied during a swap. There are default premium rates and peer-specific premium rates.

### Get Default Premium Rate

To get the default premium rate, use the following command:

For CLN:
```bash
lightning-cli peerswap-getglobalpremiumrate [btc|lbtc] [swap_in|swap_out]
```

For LND:
```bash
pscli getglobalpremiumrate --asset [btc|lbtc] --operation [swap_in|swap_out]
```

### Update Default Premium Rate

To set the default premium rate, use the following command:

For CLN:
```bash
lightning-cli peerswap-updateglobalpremiumrate [btc|lbtc] [swap_in|swap_out] [premium_rate_ppm]
```

For LND:
```bash
pscli updateglobalpremiumrate --asset [btc|lbtc] --operation [swap_in|swap_out] --rate [premium_rate_ppm]
```

### Get Peer-Specific Premium Rate

To get the premium rate for a specific peer, use the following command:

For CLN:
```bash
lightning-cli peerswap-getpremiumrate [peer_id] [BTC|LBTC] [SWAP_IN|SWAP_OUT]
```

For LND:
```bash
pscli getpeerpremiumrate --node_id [node_id] --asset [BTC|LBTC] --operation [SWAP_IN|SWAP_OUT]
```

### Update Peer-Specific Premium Rate

To set the premium rate for a specific peer, use the following command:

For CLN:
```bash
lightning-cli peerswap-updatepremiumrate [peer_id] [BTC|LBTC] [SWAP_IN|SWAP_OUT] [premium_rate_ppm]
```

For LND:
```bash
pscli updatepremiumrate --node_id [node_id] --asset [BTC|LBTC] --operation [SWAP_IN|SWAP_OUT] --rate [premium_rate_ppm]
```

### Delete Peer-Specific Premium Rate

To delete the premium rate for a specific peer, use the following command:

For CLN:
```
lightning-cli peerswap-deletepremiumrate [peer_id] [BTC|LBTC] [SWAP_IN|SWAP_OUT]
```

For LND:
```bash
pscli deletepeerpremiumrate --node_id [node_id] --asset [BTC|LBTC] --operation [SWAP_IN|SWAP_OUT]
```

## Misc

`listpeers` - A command that returns peers that support the PeerSwap protocol. It also gives statistics about received and sent swaps to a peer.

Note: peers may optionally advertise `channel_adjacency` as part of their regular poll messages. This can be used as an *advisory* hint for 2-hop discovery (e.g. find candidate swap peers behind an intermediary). The data is not trusted and may be stale; the actual swap still uses the 2-hop discovery step during negotiation. In `channel_adjacency`, `short_channel_id` is emitted in `"x"`-style (e.g. `"1x2x3"`). (Legacy name: `neighbors_ad`.)

Example output:
```bash
[
   {
      "node_id": "<peer pubkey>",
      "swaps_allowed": true,
      "supported_assets": ["BTC", "LBTC"],
      "channel_adjacency": {
         "schema_version": 1,
         "public_channels_only": true,
         "max_neighbors": 20,
         "truncated": false,
         "neighbors": [
            {
               "node_id": "<neighbor pubkey>",
               "channels": [
                  {
                     "channel_id": 1234567890,
                     "short_channel_id": "1x2x3",
                     "active": true
                  }
               ]
            }
         ]
      },
      "channels": [
         {
            "channel_id": 1234567890,
            "short_channel_id": "1:2:3",
            "local_balance": 7397932,
            "remote_balance": 2602068,
            "active": true
         }
      ],
      "as_sender": {
         "swaps_out": 2,
         "swaps_in": 1,
         "sats_out": 5300000,
         "sats_in": 302938
      },
      "as_receiver": {
         "swaps_out": 1,
         "swaps_in": 0,
         "sats_out": 2400000,
         "sats_in": 0
      },
      "paid_fee": 6082
   }
]
```

`listswaps [detailed bool (optional)]` - A command that lists all swaps. If _detailed_ is set the output shows the swap data as it is saved in the database

`listactiveswaps` - List all ongoing swaps, useful to track swaps when upgrading PeerSwap

`listswaprequests` - Lists rejected swaps requested by peer nodes.

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

`getswap` - A command that returns the swap with _swapid_
For CLN:
`lightning-cli peerswap-getswap [swapid]` 
For LND:
`pscli getswap --id [swapid]`


`reloadpolicy` - Updates the changes made to the policy file
For CLN:
`lightning-cli peerswap-reloadpolicy` 
For LND:
`pscli reloadpolicy`

`addpeer` - Adds a peer to the allowlist file
For CLN:
`lightning-cli peerswap-addpeer [peer_pubkey]` - Adds a peer to the allowlist file
For LND:
`pscli addpeer --peer_pubkey [peer_pubkey]`


`removepeer` - Remove a peer from the allowlist file
For CLN:
`lightning-cli peerswap-removepeer [peer_pubkey]`
For LND:
`pscli removepeer --peer_pubkey [peer_pubkey]`


`allowswaprequests` - Sets whether peerswap should allow new swap requests.
For CLN:
`lightning-cli peerswap-allowswaprequests [bool] ## 1 to allow, 0 to disallow`
For LND:
`pscli allowswaprequests --allow_swaps=[bool] ## true to allow, false to disallow`

## transaction labels
To make the related transactions identifiable, peerswap sets the label.
The label of the on-chain transaction corresponding to the swap is set as follows.

* peerswap -- Opening(swap id=b171ee)
* peerswap -- ClaimByCoop(swap id=b171ee)
* peerswap -- ClaimByCsv(swap id=b171ee)
* peerswap -- ClaimByInvoice(swap id=b171ee)

The way the label is set up in each wallet is different.

For CLN:
Currently, it is not possible to set a label on the cln wallet

For LND:
To check the label attached to a transaction use `lncli listchaintxns`.

For LBTC transactions, elementsd `SetLabel` will attach a label to the associated address.
