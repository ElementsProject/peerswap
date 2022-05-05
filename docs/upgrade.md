### Upgrading PeerSwap

Prior to upgrading PeerSwap you must first be certain no swaps are currently in progress.

To check for active swaps run:

 - lnd: `pscli listactiveswaps`
 - cln: `lightning-cli peerswap-listactiveswaps`

If no swaps are returned you can safely upgrade peerswap.

After you have upgraded your binaries you can run these commands to restart peerswap (customize to the actual binary path).

 - lnd: `pscli stop; /PATH/TO/peerswapd`
 - cln: `lightning-cli plugin stop peerswap-plugin; lightning-cli plugin start /PATH/TO/peerswap-plugin`

Restarting PeerSwap does not affect the accompanying CLN or LND node.

#### Temporarily disable new incoming swap requests

If you want to upgrade PeerSwap but are currently waiting for active swaps to complete you may want to temporarily disable acceptance of new incoming swap requests.

 - lnd: `pscli allowswaprequests false`
 - cln: `lightning-cli peerswap-allowswaprequests false`

To enable swaps again run: 

 - lnd: `pscli allowswaprequests true`
 - cln: `lightning-cli peerswap-allowswaprequests true`

To display the current setting run:

 - lnd: `pscli allowswaprequests`
 - cln: `lightning-cli peerswap-allowswaprequests`

Note: `allowswaprequests` will be enabled every time you start or restart PeerSwap.

#### Upgrade failures

PeerSwap has a safety feature where it will not upgrade the database format if it sees active swaps. If you have active swaps running and try to upgrade, peerswap will not start up. You should see an error message in your logs. This allows you to downgrade PeerSwap to the older version in order to complete previous swaps.
