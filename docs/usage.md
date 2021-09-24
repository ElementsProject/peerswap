# Usage guide

PeerSwap is a Peer To Peer atomic swap plugin for lightning nodes. It allows for channel rebalincing via atomic swaps with onchain coins. Supported blockchains:

- btc (bitcoin)
- l-btc (liquid)

### Build

To build the peerswap plugin a [golang](https://golang.org/doc/install) installation is needed.

Clone the repository and build the plugin

```bash
git clone git@github.com:sputn1ck/peerswap.git && \
cd peerswap && \
make release
```

### Policy

To ensure that only trusted nodes can send a peerswap request to your node it is necessary to create a policy in the lightning config dir (`~/lightning/policy.conf`) file in which the trusted nodes are specified. Change the following to your needs, replacing the _\<trusted node\>_ flag.

```bash
# ~/lightning/policy.conf
whitelisted_peers=<trusted node1>
whitelisted_peers=<trusted node2>
```

__WARNING__: One could also set the `accept_all_peers=1` policy to ignore the whitelist and allow for all peers to send swap requests.

### Run (Clightning)

start the c-lightning daemon with the following config flags

```bash
lightningd --daemon \
        --plugin=$HOME/peerswap/peerswap \
        --peerswap-liquid-rpchost=http://localhost \
        --peerswap-liquid-rpcport=18884 \
        --peerswap-liquid-rpcuser=admin1 \
        --peerswap-liquid-rpcpassword=123 \
        --peerswap-liquid-network=testnet \
        --peerswap-liquid-rpcwallet=swap \
        --peerswap-policy-path=$HOME/lightning/policy.conf
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
