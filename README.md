![peerswap logo](./docs/img/peerswap-logo.png)
# PeerSwap

*Disclaimer: PeerSwap is beta-grade software.*

*We currently only recommend using PeerSwap with small balances or on signet/testnet*

*THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.*

PeerSwap is a Peer To Peer atomic swap plugin for lightning nodes.

It allows rebalancing of your channels using btc with your nodes wallet or using l-btc on the Liquid sidechain with an external Liquid installation.

* [Project Status](#project-status)
* [Getting Started](#getting-started)
    * [Installation](#installation)
    * [Usage](#usage)
    * [Upgrading](#upgrading)
* [Further Information](#further-information)
    * [FAQ](#faq)
    * [Signet Testing](#signet-testing)
	* [Development](#development)

## Project Status

PeerSwap is beta-grade software that can be run as a [c-lightning](https://github.com/ElementsProject/lightning) plugin or as a standalone daemon/cli with [LND](https://github.com/lightningnetwork/lnd)

As we don't have a proven fee model for swaps yet, we only allow swaps with allowlisted peers.

PeerSwap allows two different types of swaps:
- [Swap-in:](./docs/peer-protocol.md#summary) trading an onchain-asset for lightning outbound liquidity
- [Swap-out:](./docs/peer-protocol.md#summary-1) trading an onchain-asset for lightning inbound liquidity

We have a detailed [Spec-draft](./docs/peer-protocol.md) available for review and reimplementation.


Join our Discord to get support and give feedback

<a href="https://discord.gg/wpNv3PG8G2" rel="some text">![Peerswap Discord](https://discordapp.com/api/guilds/905126649224388629/widget.png?style=banner2)</a>

## Getting Started

### Setup
you can use peerswap with lnd and cln:

To run peerswap as a c-lightning plugin see the [c-lightning setup guide](./docs/setup_cln.md)

To run peerswap as a standalone daemon with lnd see the [lnd setup guide](./docs/setup_lnd.md)


### Usage

See the [Usage guide](./docs/usage.md) for instructions on how to use PeerSwap.

### Upgrading
See the [Upgrade guide](./docs/upgrade.md) for instructions to safely upgrade your PeerSwap binary.


## Further Information
### FAQ

* What is the difference between BTC and L-BTC Swaps?
  * ![btc vs l-btc](./docs/img/btc_lbtc.png)
* Why should use PeerSwap instead of [Loop](https://lightning.engineering/loop/), [Boltz](https://boltz.exchange/) or other centralized swap providers?
  * Centralized swap providers rely on multi-hop payments in order to route the payment over the Lightning Network. This makes them less reliant (and more costly) than direct swaps with peers. PeerSwap is also the only swaping service that allows swaps with liquid bitcoin.

* What is the difference between [splicing](https://github.com/lightning/bolts/pull/863) and PeerSwap?
  * It is very simple and it already works today without changes to the LN protocol. Splicing also requires a change of the channel capacity. Also only peerswap allows swaps with liquid bitcoin.

* What is the difference between [liquidity-ads](https://github.com/lightning/bolts/pull/878) and PeerSwap?
  * Liquidity Ads are only for the initial channel creation. PeerSwap allows for rebalancing channels that are already active.

* Why should I do a `swap-in` vs opening a new channel?
  * If you want to leave the old channel open, opening a new channel is in fact cheaper than a `swap-in`. The advantage of a `swap-in` comes with using liquid, as it allows for new outbound liquidity in 2 minutes.

* Running Liquid is a bit much for me, do you have anything planned?
  * We will provide a light wallet using [Blockstream Green](https://github.com/Blockstream/green) in the future



### Signet Testing

#### c-lightning
For a c-lightning bitcoin-signetnet / liquid-testnet setup guide see this [guide](./docs/signetguide_clightning.md)

#### lnd
For a lnd bitcoin-signetnet / liquid-testnet setup guide see this [guide](./docs/signetguide_lnd.md)

### Development

PeerSwap uses the [nix](https://nixos.org/download.html) package manager for a simple development environment
In order to start hacking, install nix, [golang](https://golang.org/doc/install) and run `nix-shell`. This will fetch all dependencies (bar golang).
