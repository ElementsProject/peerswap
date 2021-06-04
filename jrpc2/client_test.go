package jrpc2

import (
	"bufio"
	"github.com/stretchr/testify/assert"
	"io"
	"log"
	"os"
	"sync"
	"testing"
	"time"
)

type ClientSubtract struct {
	Minuend    int
	Subtrahend int
}

func (s *ClientSubtract) Name() string {
	return "subtract"
}

func TestClientParsing(t *testing.T) {
	// set up a server with a function we can call
	s, in, out := setupServer(t)
	s.Register(&Subtract{}) // defined in jsonrpc_cases_test.go
	// setup client
	client := NewClient()
	go client.StartUp(in, out)

	answer, err := subtract(client, 8, 2)
	assert.Nil(t, err)
	assert.Equal(t, 6, answer)
}

func TestClientNoMethod(t *testing.T) {
	// set up a server with a function we can call
	_, in, out := setupServer(t)
	// setup client
	client := NewClient()
	go client.StartUp(in, out)

	answer, err := subtract(client, 8, 2)
	assert.Equal(t, "-32601:Method not found", err.Error())
	assert.Equal(t, 0, answer)
}

// send a non-parseable response back
func TestClientBadResponse(t *testing.T) {
	// set up a server with a function we can call
	_, in, out := setupServer(t)
	// setup client
	client := NewClient()
	go client.StartUp(in, out)

	answer, err := subtract(client, 8, 2)
	assert.Equal(t, "-32601:Method not found", err.Error())
	assert.Equal(t, 0, answer)
}

func TestClientIncomingInvalidJson(t *testing.T) {
	in, out, _, serverOut := setupWritePipes(t)

	client := NewClient()
	client.SetTimeout(1)
	go client.StartUp(in, out)

	logs := overrideLogger(t)
	defer resetLogger()

	ok := make(chan bool, 1)
	go func(client *Client, ok chan bool) {
		_, err := subtract(client, 5, 1)
		assert.Equal(t, "Request timed out", err.Error())
		ok <- true
	}(client, ok)
	// write junk to the client
	junk := `{"jsonrpc":"2.0"}`

	writer := bufio.NewWriter(serverOut)
	writer.Write([]byte(junk))
	writer.Flush()

	// make sure we've heard back from the client side
	select {
	case <-ok:
		buf := make([]byte, 1024)
		n, _ := logs.Read(buf)
		// check that we failed for a reason
		assert.Equal(t, "Must send either a result or an error in a response\n", string(buf[20:n]))
		assert.Equal(t, false, client.IsUp())
	case <-time.After(4 * time.Second):
		t.Logf("test timed out after %d", 4)
		t.Fail()
	}
}

type ServerSubtractString struct {
	Minuend    int
	Subtrahend int
}

func (s *ServerSubtractString) Name() string {
	return "subtract"
}

func (s *ServerSubtractString) New() interface{} {
	return &ServerSubtractString{}
}

func (s *ServerSubtractString) Call() (Result, error) {
	return string(s.Minuend - s.Subtrahend), nil
}

// send a response with a result of a different type than
// what's expected
func TestClientWrongResponseType(t *testing.T) {
	// set up a server with a function we can call
	s, in, out := setupServer(t)
	s.Register(&ServerSubtractString{})

	client := NewClient()
	go client.StartUp(in, out)

	answer, err := subtract(client, 8, 2)
	assert.Equal(t, "json: cannot unmarshal string into Go value of type int", err.Error())
	assert.Equal(t, 0, answer)
}

// send a response back with the wrong id
func TestClientNoId(t *testing.T) {
	in, out, serverIn, serverOut := setupWritePipes(t)

	logs := overrideLogger(t)
	defer resetLogger()

	client := NewClient()
	client.SetTimeout(1)
	go client.StartUp(in, out)

	ok := make(chan bool, 1)
	// write out the subtract request
	go func(client *Client, ok chan bool) {
		_, err := subtract(client, 5, 1)
		assert.Equal(t, "Request timed out", err.Error())
		ok <- true
	}(client, ok)

	// read out from pipe
	reader := bufio.NewReader(serverIn)
	resp, err := reader.ReadString('\n')
	assert.Nil(t, err)
	assert.Equal(t, "{\"jsonrpc\":\"2.0\",\"method\":\"subtract\",\"params\":{\"minuend\":5,\"subtrahend\":1},\"id\":1}\n", resp)

	jsonback := "{\"jsonrpc\":\"2.0\",\"result\":22,\"id\":2}\n\n"
	// write out response with wrong id
	writer := bufio.NewWriter(serverOut)
	writer.Write([]byte(jsonback))
	writer.Flush()

	// make sure we've heard back from the client side
	select {
	case <-ok:
		buf := make([]byte, 1024)
		n, _ := logs.Read(buf)
		// check that no return for id logged
		assert.Equal(t, "No return channel found for response with id 2\n", string(buf[20:n]))
		return
	case <-time.After(10 * time.Second):
		t.Errorf("test timed out after %d", 10)
	}
}

// double check that we're sending out requests with
// incremented ids
func TestClientCheckIdIncrement(t *testing.T) {
	in, out, serverIn, _ := setupWritePipes(t)
	client := NewClient()
	client.SetTimeout(1)
	go client.StartUp(in, out)
	var wg sync.WaitGroup
	wg.Add(2)
	// write out the subtract request
	go func(client *Client) {
		defer wg.Done()
		subtract(client, 5, 1)
	}(client)
	go func(client *Client) {
		defer wg.Done()
		subtract(client, 5, 1)
	}(client)

	// read out from pipe
	reader := bufio.NewReader(serverIn)
	resp, err := reader.ReadString('\n')
	assert.Nil(t, err)
	assert.Equal(t, "{\"jsonrpc\":\"2.0\",\"method\":\"subtract\",\"params\":{\"minuend\":5,\"subtrahend\":1},\"id\":1}\n", resp)
	// eat the extra \n between lines
	reader.ReadString('\n')
	resp, err = reader.ReadString('\n')
	assert.Nil(t, err)
	assert.Equal(t, "{\"jsonrpc\":\"2.0\",\"method\":\"subtract\",\"params\":{\"minuend\":5,\"subtrahend\":1},\"id\":2}\n", resp)

	wg.Wait()
}

// test that tihngs get shut down!
func TestClientShutdown(t *testing.T) {
	in, out, serverIn, serverOut := setupWritePipes(t)

	client := NewClient()
	client.SetTimeout(1)
	go client.StartUp(in, out)

	ok := make(chan bool, 1)
	// write out the subtract request
	go func(client *Client, ok chan bool) {
		resp, err := subtract(client, 5, 1)
		assert.NotNil(t, err)
		assert.Equal(t, "Pipe closed unexpectedly, nil result", err.Error())
		assert.Equal(t, 0, resp)
		ok <- true
	}(client, ok)

	// read out from pipe
	reader := bufio.NewReader(serverIn)
	resp, err := reader.ReadString('\n')
	assert.Nil(t, err)
	assert.Equal(t, "{\"jsonrpc\":\"2.0\",\"method\":\"subtract\",\"params\":{\"minuend\":5,\"subtrahend\":1},\"id\":1}\n", resp)

	client.Shutdown()
	jsonback := "{\"jsonrpc\":\"2.0\",\"result\":22,\"id\":1}\n\n"
	writer := bufio.NewWriter(serverOut)
	writer.Write([]byte(jsonback))
	writer.Flush()

	// make sure we've heard back from the client side
	select {
	case <-ok:
		return
	case <-time.After(10 * time.Second):
		t.Errorf("test timed out after %d", 10)
	}
}

// a notification should:
//  - not have an id
//  - return immediately
func TestClientNotification(t *testing.T) {
	in, out, serverIn, _ := setupWritePipes(t)

	client := NewClient()
	client.SetTimeout(1)
	go client.StartUp(in, out)

	err := client.Notify(&ClientSubtract{5, 1})
	assert.Nil(t, err)

	// read out from pipe
	reader := bufio.NewReader(serverIn)
	resp, err := reader.ReadString('\n')
	assert.Nil(t, err)
	assert.Equal(t, "{\"jsonrpc\":\"2.0\",\"method\":\"subtract\",\"params\":{\"minuend\":5,\"subtrahend\":1}}\n", resp)
}

func TestClientNoResponse(t *testing.T) {
	in, out, _, _ := setupWritePipes(t)

	client := NewClient()
	client.SetTimeout(1)
	go client.StartUp(in, out)

	// write out the subtract request
	_, err := subtract(client, 5, 1)
	assert.Equal(t, "Request timed out", err.Error())
}

func subtract(client *Client, minuend, subtrahend int) (int, error) {
	var response int
	err := client.Request(&ClientSubtract{minuend, subtrahend}, &response)
	return response, err
}

func overrideLogger(t *testing.T) io.Reader {
	logIn, logOut := io.Pipe()
	log.SetOutput(logOut)
	return logIn
}

func resetLogger() {
	log.SetOutput(os.Stdout)
}

func setupServer(t *testing.T) (server *Server, in, out *os.File) {
	serverIn, out, err := os.Pipe()
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	in, serverOut, err := os.Pipe()
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	server = NewServer()
	go server.StartUp(serverIn, serverOut)
	return server, in, out
}

func setupWritePipes(t *testing.T) (in, out, serverIn, serverOut *os.File) {
	serverIn, out, err := os.Pipe()
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	in, serverOut, err = os.Pipe()
	if err != nil {
		t.Log(err)
		t.FailNow()
	}

	return in, out, serverIn, serverOut
}
