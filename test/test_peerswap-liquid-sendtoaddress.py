from pyln.testing.fixtures import *
from pyln.testing.utils import wait_for
from util.utils import (
    elementsd,
    get_plugin_options,
    get_random_string,
    has_liquid_balance,
    BURN_ADDR,
    FEE,
    liquid_balance_changed,
    with_liquid_generate,
)

os.environ["TEST_DEBUG"] = "1"
os.environ["SLOW_MACHINE"] = "1"


def test_sendtoaddres(elementsd, node_factory):
    options = [
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        ),
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        ),
    ]

    nodes = node_factory.get_nodes(2, opts=options)
    nodes[0].daemon.wait_for_log("peerswap initialized")
    nodes[1].daemon.wait_for_log("peerswap initialized")

    # send liquid to node 1
    addrs = [x.rpc.call("peerswap-liquid-getaddress")["liquid_address"] for x in nodes]
    elementsd.rpc.sendtoaddress(addrs[0], 0.1, "", "", False, False, 1, "UNSET")
    # elementsd.rpc.generatetoaddress(10, addrs[0])
    elementsd.rpc.generatetoaddress(1, BURN_ADDR)
    # wait_for(lambda: has_liquid_balance(nodes[0], 10000000))

    # check balances
    balances = [x.rpc.call("peerswap-liquid-getbalance")["liquid_balance_sat"] for x in nodes]
    assert balances[0] == 10000000
    assert balances[1] == 0

    # send liquid from 0 to 1
    send_amt = 5 * 10 ** 6
    nodes[0].rpc.call(
        "peerswap-liquid-sendtoaddress", {"address": addrs[1], "amount_sat": send_amt}
    )
    wait_for(
        lambda: with_liquid_generate(
            elementsd,
            1,
            lambda: liquid_balance_changed(nodes[1], balances[1] ),
        )
    )

    # check balances
    balances = [x.rpc.call("peerswap-liquid-getbalance")["liquid_balance_sat"] for x in nodes]
    assert balances[0] == 10000000 - send_amt - 2491
    assert balances[1] == send_amt
