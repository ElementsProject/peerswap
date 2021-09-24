import logging
import os
import random
from os import path
from string import ascii_lowercase
from bitcoin.rpc import JSONRPCError
from bitcoin.rpc import RawProxy as BitcoinProxy
from ephemeral_port_reserve import reserve
from pyln.testing.fixtures import directory, teardown_checks, bitcoind
from pyln.testing.utils import (
    TIMEOUT,
    LightningNode,
    BITCOIND_CONFIG,
    SimpleBitcoinProxy,
    TailableProc,
    wait_for,
    write_config,
    BitcoinRpcProxy,
    ElementsD,
)

import pytest


FEE = 1386
BURN_ADDR = "ert1qfkht0df45q00kzyayagw6vqhfhe8ve7z7wecm0xsrkgmyulewlzqumq3ep"


def has_liquid_balance(node: LightningNode, amt: int):
    balance = node.rpc.call("peerswap-liquid-getbalance")
    return balance == amt


def has_blockcount(node: ElementsD, n: int):
    return node.rpc.getblockcount() >= n


def with_generate(node, blocks, success):
    if success():
        return True
    node.rpc.generatetoaddress(blocks, BURN_ADDR)
    return False


def with_liquid_generate(node: ElementsD, blocks: int, test, *args, **kwargs):
    print("ARGS: {}".format(args))
    if test(*args, **kwargs):
        return True
    node.rpc.generatetoaddress(blocks, BURN_ADDR)
    return False


def channel_balance_changed(node: LightningNode, before: int):
    funds = node.rpc.call("listfunds")["channels"][0]["channel_sat"]
    if funds != before:
        return True
    return False


def liquid_balance_changed(node: LightningNode, before: int):
    funds = node.rpc.call("peerswap-liquid-getbalance")
    if funds != before:
        return True
    return False


def has_log(node: TailableProc, regexs):
    logging.debug("Waiting for {} in the logs".format(regexs))
    exs = [re.compile(r) for r in regexs]
    pos = node.logsearch_start
    for r in exs.copy():
        node.logsearch_start = pos + 1
        if r.search(node.logs[pos]):
            logging.debug("Found {}} in logs".format(r))
            exs.remove(r)
            return True
    return False


def has_current_state(node: LightningNode, state: str):
    st = node.rpc.call("peerswap-listswaps")
    return st[0]["Current"] == state


def get_plugin_options(walletname, rpcport, path_to_plugin):
    return {
        "plugin": path_to_plugin,
        "peerswap-liquid-rpchost": "http://localhost",
        "peerswap-liquid-rpcport": rpcport,
        "peerswap-liquid-rpcuser": "rpcuser",
        "peerswap-liquid-rpcpassword": "rpcpass",
        "peerswap-liquid-network": "regtest",
        "peerswap-liquid-rpcwallet": walletname,
    }


def get_random_string(length):
    # choose from all lowercase letter
    result_str = "".join(random.choice(ascii_lowercase) for i in range(length))
    return result_str


def write_policy_file(dir, policy):
    with open(path.join(dir, "policy.conf"), "w+") as policy_file:
        policy_file.write(policy)


def add_policy_path_to_options(node: LightningNode):
    node.daemon.opts.update(
        {"peerswap-policy-path": path.join(node.daemon.lightning_dir, "policy.conf")}
    )


class SimpleBitcoinProxy:
    """Wrapper for BitcoinProxy to reconnect.

    Long wait times between calls to the Bitcoin RPC could result in
    `bitcoind` closing the connection, so here we just create
    throwaway connections. This is easier than to reach into the RPC
    library to close, reopen and reauth upon failure.
    """

    def __init__(
        self, btc_conf_file, service_url=None, service_port=None, *args, **kwargs
    ):
        self.__btc_conf_file__ = btc_conf_file
        self.service_url = service_url
        self.service_port = service_port

    def __getattr__(self, name):
        if name.startswith("__") and name.endswith("__"):
            # Python internal stuff
            raise AttributeError

        # Create a callable to do the actual call
        proxy = BitcoinProxy(
            service_url=self.service_url,
            service_port=self.service_port,
            btc_conf_file=self.__btc_conf_file__,
        )

        def f(*args):
            logging.debug(
                "Calling {name} with arguments {args}".format(name=name, args=args)
            )
            res = proxy._call(name, *args)
            logging.debug(
                "Result for {name} call: {res}".format(
                    name=name,
                    res=res,
                )
            )
            return res

        # Make debuggers show <function bitcoin.rpc.name> rather than <function
        # bitcoin.rpc.<lambda>>
        f.__name__ = name
        return f


class ElementsD(TailableProc):
    def __init__(self, bitcoind, elements_dir="/tmp/bitcoind-test", rpcport=None):
        TailableProc.__init__(self, elements_dir, verbose=True)

        if rpcport is None:
            rpcport = reserve()

        self.elements_dir = elements_dir
        self.rpcport = rpcport
        self.prefix = "elementsd"
        self.wallet = "liquidwallet"

        config = {}
        config["chain"] = "liquidregtest"

        regtestdir = os.path.join(elements_dir, config["chain"])
        if not os.path.exists(regtestdir):
            os.makedirs(regtestdir)

        # COMMANDS
        self.cmd_line = [
            "elementsd",
            "-datadir={}".format(elements_dir),
            "-wallet={}".format(self.wallet),
        ]

        conf_file = os.path.join(elements_dir, "elements.conf")
        ELEMENTSD_CONF = {
            "chain": config["chain"],
            "listen": 1,
        }

        ELEMENTSD_REGTEST = {
            "rpcport": self.rpcport,
            "rpcuser": "rpcuser",
            "rpcpassword": "rpcpass",
            "mainchainrpcport": bitcoind.rpcport,
            "mainchainrpcuser": "rpcuser",
            "mainchainrpcpassword": "rpcpass",
            "initialfreecoins": 2100000000000000,
            "fallbackfee": BITCOIND_CONFIG["fallbackfee"],
        }

        write_config(
            conf_file, ELEMENTSD_CONF, ELEMENTSD_REGTEST, section_name=config["chain"]
        )
        self.conf_file = conf_file
        # self.rpc = AuthServiceProxy("http://%s:%s@127.0.0.1:%s" % ('rpcuser', 'rpcpass', self.rpcport))
        # self.rpc = SimpleBitcoinProxy(btc_conf_file=self.conf_file)
        self.rpc = SimpleBitcoinProxy(
            service_url="http://rpcuser:rpcpass@127.0.0.1:{}/wallet/{}".format(
                self.rpcport, self.wallet
            ),
            btc_conf_file=self.conf_file,
        )
        self.proxies = []

    def start(self):
        TailableProc.start(self)
        self.wait_for_log("Done loading", timeout=TIMEOUT)

        logging.info("ElementsD started")
        # try:
        #     # self.rpc.createwallet(self.wallet)
        #     # self.rpc = SimpleBitcoinProxy(service_url="http://rpcuser:rpcpass@127.0.0.1:{}/wallet/{}".format(self.rpcport, self.wallet), btc_conf_file=self.conf_file)
        # except JSONRPCError:
        #     self.wallet = self.rpc.loadwallet()["name"]

    def stop(self):
        for p in self.proxies:
            p.stop()
        self.rpc.stop()
        return TailableProc.stop(self)

    def get_proxy(self):
        proxy = BitcoinRpcProxy(self)
        self.proxies.append(proxy)
        proxy.start()
        return proxy

    # wait_for_mempool can be used to wait for the mempool before generating blocks:
    # True := wait for at least 1 transation
    # int > 0 := wait for at least N transactions
    # 'tx_id' := wait for one transaction id given as a string
    # ['tx_id1', 'tx_id2'] := wait until all of the specified transaction IDs
    def generate_block(self, numblocks=1, wait_for_mempool=0):
        if wait_for_mempool:
            if isinstance(wait_for_mempool, str):
                wait_for_mempool = [wait_for_mempool]
            if isinstance(wait_for_mempool, list):
                wait_for(
                    lambda: all(
                        txid in self.rpc.getrawmempool() for txid in wait_for_mempool
                    )
                )
            else:
                wait_for(lambda: len(self.rpc.getrawmempool()) >= wait_for_mempool)

        mempool = self.rpc.getrawmempool()
        logging.debug(
            "Generating {numblocks}, confirming {lenmempool} transactions: {mempool}".format(
                numblocks=numblocks,
                mempool=mempool,
                lenmempool=len(mempool),
            )
        )

        # As of 0.16, generate() is removed; use generatetoaddress.
        return self.rpc.generatetoaddress(numblocks, self.rpc.getnewaddress())

    def simple_reorg(self, height, shift=0):
        """
        Reorganize chain by creating a fork at height=[height] and re-mine all mempool
        transactions into [height + shift], where shift >= 0. Returns hashes of generated
        blocks.

        Note that tx's that become invalid at [height] (because coin maturity, locktime
        etc.) are removed from mempool. The length of the new chain will be original + 1
        OR original + [shift], whichever is larger.

        For example: to push tx's backward from height h1 to h2 < h1, use [height]=h2.

        Or to change the txindex of tx's at height h1:
        1. A block at height h2 < h1 should contain a non-coinbase tx that can be pulled
           forward to h1.
        2. Set [height]=h2 and [shift]= h1-h2
        """
        hashes = []
        fee_delta = 1000000
        orig_len = self.rpc.getblockcount()
        old_hash = self.rpc.getblockhash(height)
        final_len = height + shift if height + shift > orig_len else 1 + orig_len
        # TODO: raise error for insane args?

        self.rpc.invalidateblock(old_hash)
        self.wait_for_log(
            r"InvalidChainFound: invalid block=.*  height={}".format(height)
        )
        memp = self.rpc.getrawmempool()

        if shift == 0:
            hashes += self.generate_block(1 + final_len - height)
        else:
            for txid in memp:
                # lower priority (to effective feerate=0) so they are not mined
                self.rpc.prioritisetransaction(txid, None, -fee_delta)
            hashes += self.generate_block(shift)

            for txid in memp:
                # restore priority so they are mined
                self.rpc.prioritisetransaction(txid, None, fee_delta)
            hashes += self.generate_block(1 + final_len - (height + shift))
        self.wait_for_log(r"UpdateTip: new best=.* height={}".format(final_len))
        return hashes

    # def getnewaddress(self):
    #     return self.rpc.getnewaddress()

    def getnewaddress(self):
        """Need to get an address and then make it unconfidential"""
        addr = self.rpc.getnewaddress()
        info = self.rpc.getaddressinfo(addr)
        return info["unconfidential"]


@pytest.fixture
def elementsd(bitcoind, directory, teardown_checks):
    elementsd = ElementsD(bitcoind, directory)

    try:
        elementsd.start()
    except Exception:
        elementsd.stop()
        raise

    info = elementsd.rpc.getnetworkinfo()

    # FIXME: include liquid-regtest in this check after elementsd has been
    # updated
    if info["version"] < 160000:
        elementsd.rpc.stop()
        raise ValueError(
            "elementsd is too old. At least version 160000 (v0.16.0)"
            " is needed, current version is {}".format(info["version"])
        )

    info = elementsd.rpc.getblockchaininfo()
    # Make sure we have some spendable funds
    if info["blocks"] < 101:
        elementsd.generate_block(101 - info["blocks"])
    elif elementsd.rpc.getwalletinfo()["balance"] < 1:
        logging.debug("Insufficient balance, generating 1 block")
        elementsd.generate_block(1)

    yield elementsd

    try:
        elementsd.stop()
    except Exception:
        elementsd.proc.kill()
    elementsd.proc.wait()
