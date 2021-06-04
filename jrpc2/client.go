package jrpc2

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// a client needs to be able to ...
// - 'call' a method which is really...
// - fire off a request
// - receive a result back (& match that result to outbound request)
// bonus round:
//    - send and receive in batches

type Client struct {
	requestQueue   chan *Request
	pending        sync.Map // map[string]chan *RawResponse
	requestCounter int64
	shutdown       bool
	timeout        time.Duration
}

func NewClient() *Client {
	client := &Client{}
	client.requestQueue = make(chan *Request)
	client.timeout = time.Duration(20)
	return client
}

func (c *Client) SetTimeout(secs uint) {
	c.timeout = time.Duration(secs)
}

func (c *Client) StartUp(in, out *os.File) {
	c.shutdown = false
	go c.setupWriteQueue(out)
	c.readQueue(in)
}

// Start up on a socket, instead of using pipes
// This method blocks. The up channel is an optional
// channel to receive  notification when the connection is set up
func (c *Client) SocketStart(socket string, up chan bool) error {
	c.shutdown = false
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return fmt.Errorf("Unable to dial socket %s:%s", socket, err.Error())
	}
	defer conn.Close()
	go func(conn net.Conn, up chan bool) {
		if up != nil {
			up <- true
		}
		c.readQueue(conn)
	}(conn, up)
	c.setupWriteQueue(conn)
	return nil
}

func (c *Client) Shutdown() {
	c.shutdown = true
	close(c.requestQueue)
	c.pending.Range(func(key, value interface{}) bool {
		v_chan, ok := value.(chan *RawResponse)
		if !ok {
			panic("value not chan *RawResponse")
		}
		close(v_chan)
		c.pending.Delete(key)
		return true
	})
	c.requestQueue = make(chan *Request)
}

func (c *Client) IsUp() bool {
	return !c.shutdown
}

func (c *Client) setupWriteQueue(outW io.Writer) {
	out := bufio.NewWriter(outW)
	defer out.Flush()
	twoNewlines := []byte("\n\n")
	for request := range c.requestQueue {
		data, err := json.Marshal(request)
		if err != nil {
			// todo: send error back to waiting response
			// iff it's got an id associated with it
			log.Println(err.Error())
			continue
		}

		if debugIO(false) {
			log.Println(string(data))
		}
		data = append(data, twoNewlines...)
		out.Write(data)
		out.Flush()
	}
}

func (c *Client) readQueue(in io.Reader) {
	decoder := json.NewDecoder(in)
	for !c.shutdown {
		var rawResp RawResponse
		if err := decoder.Decode(&rawResp); err == io.EOF {
			c.Shutdown()
			break
		} else if err != nil {
			log.Print(err.Error())
			break
		}
		go processResponse(c, &rawResp)
	}

	// there's a problem with the input, shutdown
	c.Shutdown()
}

func processResponse(c *Client, resp *RawResponse) {
	// the response should have an ID
	if resp.Id == nil || resp.Id.Val() == "" {
		// no id means there's no one listening
		// for this to come back through ...
		log.Printf("No Id provided %v", resp)
		return
	}

	id := resp.Id.Val()
	// look up 'reply channel' via the
	// client (should have a registry of
	// resonses that are waiting...)
	respChan, exists := c.pending.Load(id)
	if !exists {
		log.Printf("No return channel found for response with id %s", id)
		return
	}
	respChan.(chan *RawResponse) <- resp
	c.pending.Delete(id)
}

// Sends a notification to the server. No response is expected,
// and no ID is assigned to the request.
func (c *Client) Notify(m Method) error {
	if c.shutdown {
		return fmt.Errorf("Client is shutdown")
	}
	req := &Request{nil, m}
	c.requestQueue <- req
	return nil
}

// Isses an RPC call. Is blocking. Times out after {timeout}
// seconds (set on client).
func (c *Client) Request(m Method, resp interface{}) error {
	if c.shutdown {
		return fmt.Errorf("Client is shutdown")
	}
	id := c.NextId()
	// set up to get a response back
	replyChan := make(chan *RawResponse, 1)
	c.pending.Store(id.Val(), replyChan)

	// send the request out
	req := &Request{id, m}
	c.requestQueue <- req

	select {
	case rawResp := <-replyChan:
		return handleReply(rawResp, resp)
	case <-time.After(c.timeout * time.Second):
		c.pending.Delete(id.Val())
		return fmt.Errorf("Request timed out")
	}
}

// Hangs until a response comes. Be aware that this may never
// terminate.
func (c *Client) RequestNoTimeout(m Method, resp interface{}) error {
	if c.shutdown {
		return fmt.Errorf("Client is shutdown")
	}
	id := c.NextId()
	// set up to get a response back
	replyChan := make(chan *RawResponse, 1)
	c.pending.Store(id.Val(), replyChan)

	// send the request out
	req := &Request{id, m}
	c.requestQueue <- req

	rawResp := <-replyChan
	return handleReply(rawResp, resp)
}

func handleReply(rawResp *RawResponse, resp interface{}) error {
	if rawResp == nil {
		return fmt.Errorf("Pipe closed unexpectedly, nil result")
	}

	// when the response comes back, it will either have an error,
	// that we should parse into an 'error' (depending on the code?)
	if rawResp.Error != nil {
		if debugIO(true) {
			log.Printf("%d:%s", rawResp.Error.Code, rawResp.Error.Message)
			log.Println(string(rawResp.Error.Data))
		}
		return rawResp.Error
	}

	if debugIO(true) {
		log.Println(string(rawResp.Raw))
	}

	// or a raw response, that we should json map into the
	// provided resp (interface)
	return json.Unmarshal(rawResp.Raw, resp)
}

// for now, use a counter as the id for requests
func (c *Client) NextId() *Id {
	val := atomic.AddInt64(&c.requestCounter, 1)
	return NewIdAsInt(val)
}
