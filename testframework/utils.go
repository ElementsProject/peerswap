package testframework

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// WaitFunc returns just a bool value to check if
// the desired conditions are met.
type WaitFunc func() bool

// WaitFunc returns a bool value to check if
// the desired conditions are met. Also returns an
// error.
type WaitFuncWithErr func() (bool, error)

// WaitFor takes a WaitFunc and checks for true every
// 100ms.
func WaitFor(f WaitFunc, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("WaitFor reached timeout with %s",
				runtime.FuncForPC(uintptr(reflect.ValueOf(f).Pointer())).Name())
		default:
			if f() {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// WaitForWithErr takes a WaitFuncWithErr and checks for true every
// 100ms.
func WaitForWithErr(f WaitFuncWithErr, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return fmt.Errorf("WaitFor reached timeout with %v", f)
		default:
			ok, err := f()
			if err != nil {
				return err
			}
			if ok {
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// RequireWaitForBalanceChange waits for a change from the before value until
// timeout. Fatals the test.
func RequireWaitForBalanceChange(t *testing.T, node LightningNode, scid string, before uint64, timeout time.Duration) {
	if err := waitForBalanceChange(t, node, scid, before, timeout); err != nil {
		t.Fatalf("expected: balance change from: %d", before)
	}
}

// AssertWaitForBalanceChange waits for a change form the before value until
// timeout. Returns false if timeout was triggered and fails the test.
func AssertWaitForBalanceChange(t *testing.T, node LightningNode, scid string, before uint64, timeout time.Duration) bool {
	err := waitForBalanceChange(t, node, scid, before, timeout)
	if err != nil {
		t.Logf("expected balance change from: %d", before)
		t.Fail()
		return false
	}
	return true
}

func waitForBalanceChange(t *testing.T, node LightningNode, scid string, before uint64, timeout time.Duration) error {
	return WaitFor(func() bool {
		current, err := node.GetChannelBalanceSat(scid)
		if err != nil {
			t.Fatalf("got err %v", err)
		}

		return current != before
	}, timeout)
}

func AssertWaitForChannelBalance(t *testing.T, node LightningNode, scid string, expected, delta float64, timeout time.Duration) bool {
	actual, err := waitForChannelBalance(t, node, scid, expected, delta, timeout)
	if err != nil {
		t.Logf("expected: %d, got: %d", uint64(expected), uint64(actual))
		t.Fail()
		return false
	}
	return true
}

func RequireWaitForChannelBalance(t *testing.T, node LightningNode, scid string, expected, delta float64, timeout time.Duration) {
	actual, err := waitForChannelBalance(t, node, scid, expected, delta, timeout)
	if err != nil {
		t.Fatalf("expected: %d, got: %d", uint64(expected), uint64(actual))
	}
}

func waitForChannelBalance(t *testing.T, node LightningNode, scid string, expected, delta float64, timeout time.Duration) (float64, error) {
	var err error
	var actual uint64
	err = WaitFor(func() bool {
		actual, err = node.GetChannelBalanceSat(scid)
		if err != nil {
			t.Fatalf("got err %v", err)
		}

		dt := float64(expected) - float64(actual)
		return !(dt > delta) && !(dt < -delta)
	}, timeout)
	return float64(actual), err
}

type PortMap struct {
	sync.Mutex
	ports map[int]struct{}
}

var usedPorts = &PortMap{ports: make(map[int]struct{})}

func GetFreePort() (port int, err error) {
	var a *net.TCPAddr
	var l *net.TCPListener

	var esc int
	for {
		if esc >= 10 {
			return 0, fmt.Errorf("could not find a free port in 10 tries/")
		}

		a, err = net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			return
		}

		l, err = net.ListenTCP("tcp", a)
		if err != nil {
			return
		}

		// Check if port is registered by us.
		usedPorts.Lock()
		if _, ok := usedPorts.ports[l.Addr().(*net.TCPAddr).Port]; !ok {
			// Not registered -> register it.
			usedPorts.ports[l.Addr().(*net.TCPAddr).Port] = struct{}{}
			usedPorts.Unlock()
			l.Close()
			return l.Addr().(*net.TCPAddr).Port, nil
		}
		// Is registered -> continue
		usedPorts.Unlock()
		l.Close()
		esc++
	}
}

func FreePort(port int) {
	usedPorts.Lock()
	defer usedPorts.Unlock()
	delete(usedPorts.ports, port)
}

func GetFreePorts(n int) ([]int, error) {
	var ports []int
	for i := 0; i < n; i++ {
		a, err := net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			return nil, err
		}

		l, err := net.ListenTCP("tcp", a)
		if err != nil {
			return nil, err
		}
		defer l.Close()
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	return ports, nil
}

func GenerateRandomString(n int) (string, error) {
	const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"
	ret := make([]byte, n)
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret), nil
}

type IdGetter interface {
	NextId() int
}

type IntIdGetter struct {
	sync.Mutex
	nextId int
}

func (i *IntIdGetter) NextId() int {
	i.Lock()
	defer i.Unlock()
	i.nextId++
	return i.nextId
}

func GenerateToLiquidWallet(node *LiquidNode, walletName string, bitcoin float64) error {
	err := SwitchWallet(node, walletName)
	if err != nil {
		return fmt.Errorf("SwitchWallet() got err %w", err)
	}

	r, err := node.Call("getnewaddress")
	if err != nil {
		return fmt.Errorf("Call(getnewaddress) got err %w", err)
	}
	addr, err := r.GetString()
	if err != nil {
		return fmt.Errorf("could not get address string from response")
	}

	err = SwitchWallet(node, node.WalletName)
	if err != nil {
		return fmt.Errorf("SwitchWallet(%s) got err %w", node.WalletName, err)
	}

	_, err = node.Rpc.Call("sendtoaddress", addr, bitcoin, "", "", false, false, 1, "UNSET")
	if err != nil {
		return fmt.Errorf("Call(sendtoaddress, %s, 10) got err %w", addr, err)
	}

	_, err = node.Rpc.Call("generatetoaddress", 6, LBTC_BURN)
	if err != nil {
		return fmt.Errorf("Call(generatetoaddress, 6, %s) got err %w", LBTC_BURN, err)
	}

	return nil
}

func SwitchWallet(node *LiquidNode, walletName string) error {
	_, err := node.Rpc.Call("loadwallet", walletName)
	if err != nil {
		return fmt.Errorf("Call(\"loadwallet\") %w", err)
	}

	node.RpcProxy.UpdateServiceUrl(fmt.Sprintf("http://127.0.0.1:%d/wallet/%s", node.RpcPort, walletName))
	return nil
}

func BalanceChannel5050(node, peer *CLightningNode, scid string) error {
	funds, err := node.Rpc.ListFunds()
	if err != nil {
		return fmt.Errorf("ListFunds() %w", err)
	}

	for _, ch := range funds.Channels {
		if ch.ShortChannelId == scid {
			// We have to split the invoices so that they succeed.
			// Todo: need a better solution here.
			amt := (ch.ChannelTotalSatoshi/2 - ch.ChannelSatoshi) / 2
			for i := 0; i < 2; i++ {
				var labelBytes = make([]byte, 5)
				_, err = rand.Read(labelBytes)
				if err != nil {
					return fmt.Errorf("rand.Read() %w", err)
				}

				inv, err := node.Rpc.Invoice(amt*1000, string(labelBytes), "move-balance")
				if err != nil {
					return fmt.Errorf("Invoice() %w", err)
				}

				_, err = peer.Rpc.PayBolt(inv.Bolt11)
				if err != nil {
					return fmt.Errorf("PayBolt() %w", err)
				}
			}

			err = WaitForWithErr(func() (bool, error) {
				funds, err := node.Rpc.ListFunds()
				if err != nil {
					return false, err
				}
				for _, ch := range funds.Channels {
					if ch.ShortChannelId == scid {
						dt := float64(ch.ChannelTotalSatoshi)/2 - float64(ch.ChannelSatoshi)
						return !(dt > 1.) && !(dt < -1.), nil
					}
				}
				return false, fmt.Errorf("channel not found %s", scid)
			}, TIMEOUT)

			if err != nil {
				return err
			}
			return nil
		}
	}
	return fmt.Errorf("channel not found %s", scid)
}

func SplitLnAddr(addr string) (string, string, int, error) {
	parts := strings.Split(addr, "@")
	if len(parts) != 2 {
		return "", "", 0, fmt.Errorf("can not split addr `@` %s", addr)
	}
	p := strings.Split(parts[1], ":")
	if len(p) != 2 {
		return "", "", 0, fmt.Errorf("can not split addr `:` %s", addr)
	}
	port, err := strconv.Atoi(p[1])
	if err != nil {
		return "", "", 0, fmt.Errorf("Atoi() %w", err)
	}
	return parts[0], p[0], port, nil
}
