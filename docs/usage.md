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
pscli swapout --channel-id [chan_id] --sat_amt [amount in sats] --asset [btc or lbtc] --premium_limit_rate_ppm [premium limit in ppm]
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

## Premium

The premium rate is the rate applied during a swap. There are default premium rates and peer-specific premium rates.

### Get Default Premium Rate

To get the default premium rate, use the following command:

For CLN:
```bash
lightning-cli peerswap-getdefaultpremiumrate [btc|lbtc] [swap_in|swap_out]
```

For LND:
```bash
pscli getdefaultpremiumrate --asset [btc|lbtc] --operation [swap_in|swap_out]
```

### Set Default Premium Rate

To set the default premium rate, use the following command:

For CLN:
```bash
lightning-cli peerswap-setdefaultpremiumrate [btc|lbtc] [swap_in|swap_out] [premium_rate_ppm]
```

For LND:
```bash
pscli updatedefaultpremiumrate --asset [btc|lbtc] --operation [swap_in|swap_out] --rate [premium_rate_ppm]
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

### Set Peer-Specific Premium Rate

To set the premium rate for a specific peer, use the following command:

For CLN:
```bash
lightning-cli peerswap-setpremiumrate [peer_id] [BTC|LBTC] [SWAP_IN|SWAP_OUT] [premium_rate_ppm]
```

For LND:
```bash
pscli updatepremiumrate --node_id [node_id] --asset [BTC|LBTC] --operation [SWAP_IN|SWAP_OUT] --rate [premium_rate_ppm]
```

## Misc

`listpeers` - A command that returns peers that support the PeerSwap protocol. It also gives statistics about received and sent swaps to a peer.

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
      "total_fee_paid": 6082,
      "swap_in_premium_rate": "100",
      "swap_out_premium_rate": "100"
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