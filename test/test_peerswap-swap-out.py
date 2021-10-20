import os
import time
from pyln.testing.fixtures import *
from pyln.testing.utils import NodeFactory, TailableProc, wait_for
from pyln.client import RpcError
from util.utils import (
    get_plugin_options,
    get_random_string,
    write_policy_file,
    has_current_state,
    add_policy_path_to_options,
    ElementsD,
    elementsd,
    has_liquid_balance,
    with_liquid_generate,
    channel_balance_changed,
    liquid_balance_changed,
    has_log,
    not_in_log,
    FEE,
    BURN_ADDR,
)

os.environ["TEST_DEBUG"] = "1"
os.environ["SLOW_MACHINE"] = "1"


def test_swap_out(elementsd: ElementsD, node_factory: NodeFactory):
    FUNDAMOUNT = 10 ** 7

    options = [{"start": True}, {"start": False}]

    options[0].update(
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        )
    )
    options[1].update(
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        )
    )

    nodes = node_factory.get_nodes(2, opts=options)

    # allowlist node 0 on node 1
    policy = "allowlisted_peers={}".format(nodes[0].info["id"])
    write_policy_file(nodes[1].daemon.lightning_dir, policy)
    add_policy_path_to_options(nodes[1])
    nodes[1].start()

    # create channel between 0 and 1
    node_factory.join_nodes(nodes, fundchannel=True, fundamount=FUNDAMOUNT)
    ch = nodes[0].rpc.listfunds()["channels"][0]
    chfunds = ch["channel_sat"]
    scid = ch["short_channel_id"]
    assert chfunds == FUNDAMOUNT

    # send liquid to node wallets
    addrs = [x.rpc.call("peerswap-liquid-getaddress") for x in nodes]
    for addr in addrs:
        elementsd.rpc.sendtoaddress(addr, 0.1, "", "", False, False, 1, "UNSET")

    elementsd.rpc.generatetoaddress(1, BURN_ADDR)
    wait_for(lambda: has_liquid_balance(nodes[0], 10000000))
    wait_for(lambda: has_liquid_balance(nodes[1], 10000000))

    balances = [x.rpc.call("peerswap-liquid-getbalance") for x in nodes]
    assert balances[0] == 10000000
    assert balances[1] == 10000000

    # swap out 5000000 sat
    swap_amt = 5 * 10 ** 6
    nodes[0].rpc.call(
        "peerswap-swap-out",
        {"amt": swap_amt, "short_channel_id": scid, "asset": "l-btc"},
    )

    # wait for fee beeing payed
    wait_for(
        lambda: with_liquid_generate(
            elementsd,
            1,
            lambda: channel_balance_changed(nodes[0], chfunds),
        )
    )
    chfunds_after_fee_payed = nodes[0].rpc.call("listfunds")["channels"][0][
        "channel_sat"
    ]
    assert chfunds_after_fee_payed - chfunds == -1 * FEE

    # wait for tx beeing broadcasted
    wait_for(
        lambda: with_liquid_generate(
            elementsd, 1, lambda: liquid_balance_changed(nodes[1], balances[1])
        )
    )
    balances_invoice_payed = [x.rpc.call("peerswap-liquid-getbalance") for x in nodes]
    assert balances_invoice_payed[0] == balances[0]
    assert balances_invoice_payed[1] == balances[1] - FEE - swap_amt

    # wait for invoice being payed
    wait_for(
        lambda: with_liquid_generate(
            elementsd,
            1,
            lambda: channel_balance_changed(nodes[0], chfunds_after_fee_payed),
        )
    )
    chfunds_after_invoice_payed = nodes[0].rpc.call("listfunds")["channels"][0][
        "channel_sat"
    ]
    assert chfunds - FEE - swap_amt == chfunds_after_invoice_payed

    # wait for claiming tx
    wait_for(
        lambda: with_liquid_generate(
            elementsd, 1, lambda: liquid_balance_changed(nodes[0], balances[0])
        )
    )
    balances_after_claim = [x.rpc.call("peerswap-liquid-getbalance") for x in nodes]
    assert balances[0] + swap_amt - 501 == balances_after_claim[0]
    assert balances[1] - swap_amt - FEE == balances_after_claim[1]


def test_swap_out_claim_cltv(elementsd: ElementsD, node_factory: NodeFactory):
    FUNDAMOUNT = 10 ** 7

    options = [{"start": True}, {"start": False}]

    options[0].update(
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        )
    )
    options[1].update(
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        )
    )

    nodes = node_factory.get_nodes(2, opts=options)

    # allowlist node 0 on node 1
    policy = "allowlisted_peers={}".format(nodes[0].info["id"])
    write_policy_file(nodes[1].daemon.lightning_dir, policy)
    add_policy_path_to_options(nodes[1])
    nodes[1].start()

    # create channel between 0 and 1
    node_factory.join_nodes(nodes, fundchannel=True, fundamount=FUNDAMOUNT)
    ch = nodes[0].rpc.listfunds()["channels"][0]
    chfunds = ch["channel_sat"]
    scid = ch["short_channel_id"]
    assert chfunds == FUNDAMOUNT

    # send liquid to node wallets
    addrs = [x.rpc.call("peerswap-liquid-getaddress") for x in nodes]
    for addr in addrs:
        elementsd.rpc.sendtoaddress(addr, 0.1, "", "", False, False, 1, "UNSET")

    elementsd.rpc.generatetoaddress(1, BURN_ADDR)
    wait_for(lambda: has_liquid_balance(nodes[0], 10000000))
    wait_for(lambda: has_liquid_balance(nodes[1], 10000000))

    balances = [x.rpc.call("peerswap-liquid-getbalance") for x in nodes]
    assert balances[0] == 10000000
    assert balances[1] == 10000000

    # swap out 5000000 sat
    swap_amt = 5 * 10 ** 6
    nodes[0].rpc.call(
        "peerswap-swap-out",
        {"amt": swap_amt, "short_channel_id": scid, "asset": "l-btc"},
    )

    # wait for fee beeing payed
    wait_for(
            lambda: channel_balance_changed(nodes[0], chfunds),
    )
    chfunds_after_fee_payed = nodes[0].rpc.call("listfunds")["channels"][0][
        "channel_sat"
    ]
    assert chfunds_after_fee_payed - chfunds == -1 * FEE

    # wait for tx beeing broadcasted
    try: 
        nodes[1].daemon.wait_for_log("Event_ActionSucceeded on State_SwapOutReceiver_SendTxBroadcastedMessage")
    except Exception as e:
        print("FAILED")
        pytest.fail()

    # check that mempool has one entry
    assert len(elementsd.rpc.getrawmempool()) == 1

    # as we want to test on csv claim, we shut down
    # the second node so that the invoice never gets
    # payed and the csv claim is performet eventually!
    nodes[0].stop()

    # assert 60 blocks difference until csv is claimed
    block_diff = 60
    blocks = elementsd.rpc.getblockcount()
    elementsd.rpc.generatetoaddress(block_diff - 1, BURN_ADDR)
    elementsd.wait_for_log("height={}".format(blocks + block_diff - 1))
    assert not_in_log(nodes[1].daemon, "Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv")
    elementsd.rpc.generatetoaddress(1, BURN_ADDR)
    nodes[1].daemon.wait_for_log("Event_ActionSucceeded on State_SwapOutReceiver_ClaimSwapCsv")
    # check expected cltv time


def test_peer_not_allowlisted(elementsd: ElementsD, node_factory: NodeFactory):
    FUNDAMOUNT = 10 ** 7

    options = [{"start": True}, {"start": False}]

    options[0].update(
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        )
    )
    options[1].update(
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        )
    )

    nodes = node_factory.get_nodes(2, opts=options)

    # do NOT allowlist node 0 on node 1
    policy = ""
    write_policy_file(nodes[1].daemon.lightning_dir, policy)
    add_policy_path_to_options(nodes[1])
    nodes[1].start()

    # create channel between 0 and 1
    node_factory.join_nodes(nodes, fundchannel=True, fundamount=FUNDAMOUNT)
    ch = nodes[0].rpc.listfunds()["channels"][0]
    chfunds = ch["channel_sat"]
    scid = ch["short_channel_id"]
    assert chfunds == FUNDAMOUNT

    # send liquid to node wallets
    addrs = [x.rpc.call("peerswap-liquid-getaddress") for x in nodes]
    for addr in addrs:
        elementsd.rpc.sendtoaddress(addr, 0.1, "", "", False, False, 1, "UNSET")

    elementsd.rpc.generatetoaddress(1, BURN_ADDR)
    wait_for(lambda: has_liquid_balance(nodes[0], 10000000))
    wait_for(lambda: has_liquid_balance(nodes[1], 10000000))

    balances = [x.rpc.call("peerswap-liquid-getbalance") for x in nodes]
    assert balances[0] == 10000000
    assert balances[1] == 10000000

    # swap out 5000000 sat
    swap_amt = 5 * 10 ** 6

    # assert error message as peer is not allowlisted
    CANCEL_MSG = "peer not allowed to request swaps"
    with pytest.raises(RpcError, match=CANCEL_MSG):
        nodes[0].rpc.call(
            "peerswap-swap-out",
            {"amt": swap_amt, "short_channel_id": scid, "asset": "l-btc"},
        )

    # wait for nodes to receive events
    nodes[0].daemon.wait_for_log("Event_OnCancelReceived on State_SwapOutSender_AwaitFeeResponse")
    nodes[1].daemon.wait_for_log("Event_ActionSucceeded on State_SendCancel")

    # assert swap canceled on both nodes
    wait_for(lambda: has_current_state(nodes[0], "State_SwapCanceled"))
    wait_for(lambda: has_current_state(nodes[1], "State_SwapCanceled"))
    res = [x.rpc.call("peerswap-listswaps") for x in nodes]
    assert res[0][0]["Data"]["CancelMessage"] == CANCEL_MSG
    assert res[1][0]["Data"]["CancelMessage"] == CANCEL_MSG

    # assert same balances as before
    balances_post_cancel = [x.rpc.call("peerswap-liquid-getbalance") for x in nodes]
    assert balances[0] == balances_post_cancel[0]
    assert balances[1] == balances_post_cancel[1]

    ch_post_cancel = nodes[0].rpc.listfunds()["channels"][0]
    chfunds_post_cancel = ch_post_cancel["channel_sat"]
    assert chfunds_post_cancel == FUNDAMOUNT


def test_accept_all_peers(elementsd: ElementsD, node_factory: NodeFactory):

    FUNDAMOUNT = 10 ** 7
    
    options = [{"start": True}, {"start": False}]

    options[0].update(
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        )
    )

    options[1].update(
        get_plugin_options(
            get_random_string(8),
            elementsd.rpcport,
            os.path.join(os.path.dirname(__file__), "../peerswap"),
        )
    )

    nodes = node_factory.get_nodes(2, opts=options)

    # accept all peers on node 1
    policy = "accept_all_peers=1"
    write_policy_file(nodes[1].daemon.lightning_dir, policy)
    add_policy_path_to_options(nodes[1])
    nodes[1].start()

    # create channel between 0 and 1
    node_factory.join_nodes(nodes, fundchannel=True, fundamount=FUNDAMOUNT)
    ch = nodes[0].rpc.listfunds()["channels"][0]
    chfunds = ch["channel_sat"]
    scid = ch["short_channel_id"]
    assert chfunds == FUNDAMOUNT

    # send liquid to node wallets
    addrs = [x.rpc.call("peerswap-liquid-getaddress") for x in nodes]
    for addr in addrs:
        elementsd.rpc.sendtoaddress(addr, 0.1, "", "", False, False, 1, "UNSET")

    elementsd.rpc.generatetoaddress(1, BURN_ADDR)
    wait_for(lambda: has_liquid_balance(nodes[0], 10000000))
    wait_for(lambda: has_liquid_balance(nodes[1], 10000000))

    balances = [x.rpc.call("peerswap-liquid-getbalance") for x in nodes]
    assert balances[0] == 10000000
    assert balances[1] == 10000000

    # swap out 5000000 sat
    swap_amt = 5 * 10 ** 6

    # assert NO error message as all peers are accepted
    nodes[0].rpc.call(
        "peerswap-swap-out",
        {"amt": swap_amt, "short_channel_id": scid, "asset": "l-btc"},
    )
    
    # assert opening tx broadcasted and tx broadcasted
    # msg sent and also awaiting confirmations on
    # receiver.
    wait_for(lambda: has_current_state(nodes[0], "State_SwapOutSender_AwaitTxConfirmation"))
    wait_for(lambda: has_current_state(nodes[1], "State_SwapOutReceiver_AwaitClaimInvoicePayment"))
