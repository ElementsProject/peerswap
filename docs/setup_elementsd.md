# Setup `elementsd`

`elementsd` is the daemon used to sync and verify the [Liquid Network](https://docs.liquid.net/docs) and is used for PeerSwap L-BTC swaps. To set up `elementsd` for PeerSwap, follow the steps here. 


## Building from source

To compile `elementsd` from source, follow the [documentation for Linux](https://github.com/ElementsProject/elements/blob/master/doc/build-unix.md).

If you would rather just download the binary instead, skip to the next section.


## Download binaries

Download the latest `elementsd` binary release [here](https://github.com/ElementsProject/elements/releases). 

Extract the archive

`tar xvf elements-*.tar.gz`

Copy the binaries to your PATH

`cp elements*/elementsd elements*/elements-cli /usr/local/bin`


## Configuring

The default data directory for `elementsd` is located in the home directory:

`~/.elements`

The config file is not created automatically. If one is created it should be placed inside the data directory as such:

`~/.elements/elements.conf`

If running `elementsd` as the same user as PeerSwap, then configuration is not needed.

Otherwise, you need to set the `rpcport`, `rpcuser`, `rpcpassword`, and other config options depending on how you're deploying, in both `elements.conf` and `peerswap.conf`.

More details:

 [For CLN](https://github.com/ElementsProject/peerswap/blob/master/docs/setup_cln.md#config-file)
 
 [For LND](https://github.com/ElementsProject/peerswap/blob/master/docs/setup_lnd.md#config-file)
 
 
 >**Note**
 
 >It's recommended to add `trim_headers=1` to the config file to reduce RAM usage by roughly 50%. However, this will also mean your node cannot help other nodes sync Liquid Network headers. This mode is not appropriate for "infrastructure" nodes which need to provide support for IBD or block/transaction propagation.


## Wallet

PeerSwap will automatically create a wallet if running as the same user as `elementsd`. 
This `elementsd` wallet, used for L-BTC transfers and swaps, is located here for mainnet:

`~/.elements/liquidv1/wallets/peerswap/wallet.dat`

For Liquid testnet, it is located here:

`~/.elements/liquidtestnet/wallets/peerswap/wallet.dat`


#### Manually operating the elementsd wallet

The `elementsd` wallet is normally automatically controlled by the PeerSwap plugin or standalone daemon (LND) when doing swaps. No user input is required. 

However, if you need to manually use the `elementsd` wallet, such as to swap with [Boltz](https://liquid.boltz.exchange/), you can use the `elements-cli` utility:


Make sure the `peerswap` wallet is loaded first:

`elements-cli loadwallet peerswap`

To create a new Liquid receiving address:

`elements-cli -rpcwallet=peerswap getnewaddress`

To send L-BTC to a Liquid address:

`elements-cli -rpcwallet=peerswap sendtoaddress [address] [amount in decimal form, e.g. 0.1 for 0.10000000 L-BTC]`

