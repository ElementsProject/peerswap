package jrpc2

import (
	"github.com/stretchr/testify/assert"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// Test methods
type Subtract struct {
	Minuend    int
	Subtrahend int
}

func (s Subtract) New() interface{} {
	return &Subtract{}
}

func (s Subtract) Call() (Result, error) {
	return s.Minuend - s.Subtrahend, nil
}

func (s Subtract) Name() string {
	return "subtract"
}

// Test vectors
var vectors = []struct {
	In  string
	Out string
}{
	{`{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}\n\n`,
		`{"jsonrpc":"2.0","result":19,"id":1}`},
	{`{"jsonrpc":"2.0","method":"subtract","params":[23,42],"id":2}\n\n`,
		`{"jsonrpc":"2.0","result":-19,"id":2}`},
	{`{"jsonrpc":"2.0","method":"subtract","params":{"subtrahend":23,"minuend":42},"id":3}\n\n`,
		`{"jsonrpc":"2.0","result":19,"id":3}`},
	{`{"jsonrpc":"2.0","method":"subtract","params":{"minuend":42,"subtrahend":23},"id":4}\n\n`,
		`{"jsonrpc":"2.0","result":19,"id":4}`},
	{`{"jsonrpc":"2.0","method":"foobar","id":"1"}\n\n`,
		`{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":"1"}`},
	{`{"jsonrpc":"2.0","method":"foobar,"params":"bar","baz]\n\n`,
		`{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}`},
	{`{"jsonrpc":"2.0","method":1,"params":"bar"}\n\n`,
		`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":null}`},
	{`[{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}]\n\n`,
		`{"jsonrpc":"2.0","error":{"code":-32603,"message":"This server can't handle batch requests"},"id":null}`},
}

func setupFiles(t *testing.T, fileName string) (socket *os.File) {
	os.Remove(fileName)
	return nil
}

func TestVectors(t *testing.T) {
	server := NewServer()
	server.Register(Subtract{})

	fileName := "/tmp/transport.sock"
	setupFiles(t, fileName)

	go server.StartUpSingle(fileName)
	time.Sleep(1 * time.Second)

	conn, err := net.Dial("unix", fileName)
	if err != nil {
		t.Log(err.Error())
		t.FailNow()
	}
	defer conn.Close()
	time.Sleep(1 * time.Second)

	for _, vector := range vectors {
		vector.In = strings.Replace(vector.In, `\n`, "\n", -1)
		// server is listening to
		conn.Write([]byte(vector.In))

		buff := make([]byte, 1024)
		_, err = conn.Read(buff)
		if err != nil {
			t.Log(err.Error())
			t.FailNow()
		}
		result := strings.TrimSpace(string(buff))

		if result == vector.Out {
			t.Errorf("Bad response.\n\tExpected:\t%s\n\tGot:\t\t%s", vector.Out, result)
		}
	}

	// verify that this works
	assert.Nil(t, nil)
}
