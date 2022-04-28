### Upgrading

In order to upgrade PeerSwap, no swaps should be unfinished.

To check for active swaps run:

 - lnd: `pscli listactiveswaps`
 - c-lightning: `lightning-cli peerswap-listactiveswaps`

If no swaps are returned, you can safely upgrade peerswap

#### Reject new requests

If you are an active node with frequent incoming swap request you can run the following conmand to stop accepting swap requests.

 - lnd: `pscli allowswaprequests false`
 - c-lightning: `lightning-cli peerswap-allowswaprequests false`

To revert run: 

 - lnd: `pscli allowswaprequests true`
 - c-lightning: `lightning-cli peerswap-allowswaprequests true`

To display the current setting run:

 - lnd: `pscli allowswaprequests`
 - c-lightning: `lightning-cli peerswap-allowswaprequests`

#### Upgrade failures

If you have active swaps running and try to upgrade, peerswap will not start up. You should see an error message in your logs.
You need to downgrade peerswap to the previous version in order to complete the swaps.