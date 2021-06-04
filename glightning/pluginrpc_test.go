package glightning

import (
	"fmt"
	"github.com/sputn1ck/liquid-loop/jrpc2"
	"testing"
)

type ParamMethod struct {
	Required string `json:"required"`
	Optional string `json:"optional,omitempty"`
}

func (p *ParamMethod) Name() string {
	return "param-test"
}

func (p *ParamMethod) New() interface{} {
	return &ParamMethod{}
}

func (p *ParamMethod) Call() (jrpc2.Result, error) {
	return fmt.Sprintf("Called with %s and [%s]", p.Required, p.Optional), nil
}

func TestManifestWithUsage(t *testing.T) {
	initFn := getInitFunc(t, func(t *testing.T, options map[string]Option, config *Config) {
		t.Error("Should not have called init when calling get manifest")
	})
	plugin := NewPlugin(initFn)
	plugin.RegisterMethod(NewRpcMethod(&ParamMethod{}, "Call a param"))

	msg := "{\"jsonrpc\":\"2.0\",\"method\":\"getmanifest\",\"id\":\"aloha\"}\n\n"
	resp := "{\"jsonrpc\":\"2.0\",\"result\":{\"options\":[],\"rpcmethods\":[{\"name\":\"param-test\",\"description\":\"Call a param\",\"usage\":\"required [optional]\"}],\"dynamic\":true,\"featurebits\":{}},\"id\":\"aloha\"}"
	runTest(t, plugin, msg, resp)
}
