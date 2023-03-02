# Docker Suggestions

Distributors, integrators, and end users are responsible for proper packaging and deployment of the PeerSwap project. The PeerSwap project is not responsible for how the code is deployed. Please refer to your local system administrator, PaaS provider, or appliance distributor. 


## Core Lightning (CLN)

All CLN plugins, including the PeerSwap CLN plugin, need to be run in the same Docker container as CLN due to how plugins interface with CLN. A custom Docker container is recommended if deploying CLN and the PeerSwap plugin.

## LND

Due to an [issue with Docker's internal networking](https://github.com/ElementsProject/peerswap/issues/167), LND and `peerswapd` also need to be run inside of the same container. There is a workaround for running with independent containers, but it requires exposing the containers to the host network with `--network=host` which is likely not a good idea security-wise. The optimal route currently is a custom container that runs both LND and `peerswapd`. 
