# Setup `lwk`

**[Liquid Wallet Kit](https://github.com/Blockstream/lwk/tree/master)** is a collection of Rust crates for [Liquid](https://liquid.net) Wallets and is used for PeerSwap L-BTC swaps.  
To set up `lwk` for PeerSwap, follow the steps here.  
lwk is currently under development and changes are being made.  
**peerswap has been tested only with [cli_0.3.0](https://github.com/Blockstream/lwk/tree/cli_0.3.0)**.

## wallet
peerswap assumes a wallet with blinding-key set in singlesig to lwk.

```sh
MNEMONIC=$(lwk_cli signer generate | jq -r .mnemonic)
lwk_cli signer load-software --mnemonic "$MNEMONIC" --signer <signer_name>
DESCRIPTOR=$(lwk_cli signer singlesig-desc --signer <signer_name> --descriptor-blinding-key slip77 --kind wpkh | jq -r .descriptor)
lwk_cli wallet load --wallet <wallet_name> -d "$DESCRIPTOR"
```


Below is an example of an appropriate descriptor.
Confidential Transactions with [SLIP-0077](https://github.com/satoshilabs/slips/blob/master/slip-0077.md), P2WPKH output with the specified xpub.
```sh
"ct(slip77(220b6575205a476aac5a8c09f497ab084c13c269a7345846e617698f9beda171),elwpkh([4cd32cc8/84h/1h/0h]tpubDDbFo41vfUWdQMSjEjYVBNgEamvzpJWWqLspuDvStJyaCXC1EKxGyvABFCbax3k5adihtmWakYokMMWV67rZMjLjSuMnHSxKmZS92gKwbNw/<0;1>/*))"
```

## server
peerswap uses lwk's json rpc, so you need to start lwk's server.  
Follow [lwk's document](https://github.com/Blockstream/lwk) to start the server.

```sh
lwk_cli server start
```

## electrum
peerswap uses [esplora-electrs](https://github.com/Blockstream/electrs) to communicate with the luqiud chain.  
By default, peerswap connects to `blockstream.info:995` used by lwk, so no configuration is needed.

If you want to use your own chain, follow the instructions in [esplora-electrs](https://github.com/Blockstream/electrs) to start Electrum JSON-RPC server.

## config file
The following settings are available
* wallet name
* signer name
* lwk endpoint : lwk jsonrpc endpoint
* electrumEndpoint : electrum JSON-RPC serverのendpoint
* network : **`liquid`, `liquid-testnet`, `liquid-regtest`**
* liquidSwaps : `true` if used

Set up in INI (.ini) File Format for lnd and in toml format for cln

### example
Example configuration in lnd
```sh
lwk.signername=signername
lwk.walletname=walletname
lwk.network=liquid
lwk.liquidswaps=true
```

Example configuration in cln
```sh
[LWK]
signername="signername"
walletname="walletname"
network="liquid"
liquidswaps=true
```