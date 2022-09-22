### Upgrading PeerSwap

Prior to upgrading PeerSwap you must first be certain no swaps are currently in progress.

To check for active swaps run:

 - lnd: `pscli listactiveswaps`
 - cln: `lightning-cli peerswap-listactiveswaps`

If no swaps are returned you can safely upgrade peerswap.

### Restarting LND peerswapd
 - lnd: `pscli stop; /PATH/TO/peerswapd`

### Restarting CLN peerswap-plugin
Due to [CLN issue #5294](https://github.com/ElementsProject/lightning/issues/5294) it is recommended that you fully restart `lightningd` in order to restart peerswap. If you do not rely on CLN to pass any config parameters to peerswap then it is safe to restart peerswap-plugin without touching CLN.


 - cln full restart: `lightning-cli stop; /PATH/TO/lightningd`
 - peerswap-plugin restart without lightningd config options: `lightning-cli plugin stop peerswap-plugin; lightning-cli plugin start /PATH/TO/peerswap-plugin`

Restarting PeerSwap does not affect the accompanying CLN or LND node.

#### Temporarily disable new incoming swap requests

If you want to upgrade PeerSwap but are currently waiting for active swaps to complete you may want to temporarily disable acceptance of new incoming swap requests.

 - lnd: `pscli allowswaprequests --allow_swaps=false`
 - cln: `lightning-cli peerswap-allowswaprequests 0`

To enable swaps again run: 

 - lnd: `pscli allowswaprequests --allow_swaps=false`
 - cln: `lightning-cli peerswap-allowswaprequests 1`

To display the current setting run:

 - lnd: `pscli reloadpolicy`
 - cln: `lightning-cli peerswap-reloadpolicy`

Note: `allowswaprequests` will be enabled every time you start or restart PeerSwap.

#### Upgrade failures

PeerSwap has a safety feature where it will not upgrade the database format if it sees active swaps. If you have active swaps running and try to upgrade, peerswap will not start up. You should see an error message in your logs. This allows you to downgrade PeerSwap to the older version in order to complete previous swaps.
