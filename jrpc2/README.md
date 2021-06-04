# jrpc2: a thin framework for making jsonrpc2 clients and servers

`jrpc2` is an almost standards compliant implementation of [JSON-RPC 2.0](https://www.jsonrpc.org/specification).

A few modifications have been made to better fit the needs of the c-lightning 
RPC, most notably that the server offers a "Notify" method.

## Some notes on architecture
A marriage of JSON and a strongly typed language such as Go is never a truly
happy affair. In order to provide the best hiding of this fact, `glightning` offers
a boutique jsonrpc2 implementation, jrpc2. 

`glightning` takes advantage of reflection to marshal and unmarshal method parameters. 

### Offering a method from the Server

To have your server respond to methods, you'll need to create a new type struct that implements the `ServerMethod` interface.

```
type ServerMethod interface {
	Method
	New() interface{}
	Call() (Result, error)
}
```

Here's an example:

```
// Declare a new Subtract method, as a struct

// The public members of this struct will be the expected parameters
// when the method is called.
type Subtract struct {
	Minuend    int	`json:"minuend"`
	Subtrahend int	`json:"subtrahend"`
}

func (s *Subtract) New() interface{} {
	return &Subtract{}
}

func (s *Subtract) Call() (jrpc2.Result, error) {
	return s.Minuend - s.Subtrahend, nil
}

// This is the name of the method!
func (s *Subtract) Name() string {
	return "subtract"
}
```

After your server method has been created, you just need to register the
method with the server, so it knows who to call when it receives a request.

```
server := jrpc2.NewServer()
server.Register(&Subtract{})
```

All that's left to do now is to start up the server on the socket or pipeset of your choice.

### Calling a method from a Client

Calling a method from the Client is much easier. You only need to pass a Method
and a result object to client, as a request.


```
type ClientSubtract struct {
	Minuend int
	Subtrahend int
}

func (s *ClientSubtract) Name() string {
	return "subtract"
}

// You can wrap the client call in a function for convenience
func subtract(c *jrpc2.Client, min, sub int) int {
	var result int	
	err := c.Request(&ClientSubtract{min,sub}, &result)
	if err != nil {
		panic(err)
	}
	return result
}

```

You can also send notifications from the client. These are JSON-RPC notifications, which means they do not include an ID and will not get a response from the server.

```
client.Notify(&ClientSubtract{min,sub})
```

## Missing Features
`jrpc2` currently does not support request batching

`jrpc2` currently does not provide an elegant mechanism for parsing extra data
that is passed back in error responses.
