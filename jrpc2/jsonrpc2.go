package jrpc2

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

const specVersion = "2.0"

const ParseError = -32700
const InvalidRequest = -32600
const MethodNotFound = -32601
const InvalidParams = -32603
const InternalErr = -32603

// ids for JSON-RPC v2 can be a string, an integer
// or null. We use the pointer type in Request to
// simulate a null value for JSON mapping; this
// struct manages the rest.  for giggles, we map all Ids
// to strings, but in the case of this being something
// that's populated from an incoming request, we need to maintain the
// 'actual' type of it so when we send it back over the
// wire, we don't confuse the other side.
type Id struct {
	intVal int64
	strVal string
}

func (id Id) MarshalJSON() ([]byte, error) {
	if id.strVal != "" {
		return json.Marshal(id.strVal)
	}
	return json.Marshal(id.intVal)
}

func (id *Id) UnmarshalJSON(data []byte) error {
	// check first character...
	if len(data) == 0 {
		return NewError(nil, ParseError, "no data provided")
	}
	switch rune(data[0]) {
	case '"':
		if data[len(data)-1] != '"' {
			return NewError(nil, ParseError, "Parse error")
		}
		id.strVal = string(data[1 : len(data)-1])
		return nil
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		val, err := strconv.ParseInt(string(data), 10, 64)
		if err != nil {
			return NewError(nil, InvalidRequest, fmt.Sprintf("Invalid Id value: %s", string(data)))
		}
		id.intVal = val
		return nil
	case '{': // objects not allowed!
		fallthrough
	case '[': // arrays not allowed!
		fallthrough
	default:
		return NewError(nil, InvalidRequest, fmt.Sprintf("Invalid Id value: %s", string(data)))
	}
}

func (id *Id) Val() string {
	return id.String()
}

func (id Id) String() string {
	if id.strVal != "" {
		return id.strVal
	}
	return strconv.FormatInt(id.intVal, 10)
}

func NewId(val string) *Id {
	return &Id{
		strVal: val,
	}
}

func NewIdAsInt(val int64) *Id {
	return &Id{
		intVal: val,
	}
}

// Models for the model gods
type Request struct {
	Id     *Id    `json:"id,omitempty"`
	Method Method `json:"-"`
}

type Method interface {
	Name() string
}

// Responses are sent by the Server
type Response struct {
	Result Result    `json:"result,omitempty"`
	Error  *RpcError `json:"error,omitempty"`
	Id     *Id       `json:"id"`
}

// RawResponses are what the client gets back
// from an RPC call.
// Leaving raw json around is kind of hacky,
// until you realize how clean it is from a parsing
// perspective
type RawResponse struct {
	Id    *Id             `json:"id"`
	Raw   json.RawMessage `json:"-"`
	Error *RpcError       `json:"error,omitempty"`
}

type Result interface{}

type RpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// provide your own object to parse this with! ehehe
func (e *RpcError) ParseData(into interface{}) error {
	return json.Unmarshal(e.Data, into)
}

func (e *RpcError) Error() string {
	return fmt.Sprintf("%d:%s", e.Code, e.Message)
}

// What we really want is the parameter values off of
// the Method object
// called on the client side
func (r *Request) MarshalJSON() ([]byte, error) {
	type Alias Request
	return json.Marshal(&struct {
		Version string                 `json:"jsonrpc"`
		Name    string                 `json:"method"`
		Params  map[string]interface{} `json:"params"`
		*Alias
	}{
		Alias:   (*Alias)(r),
		Params:  GetNamedParams(r.Method),
		Version: specVersion,
		Name:    r.Method.Name(),
	})
}

type CodedError struct {
	Id   *Id
	Code int
	Msg  string
}

func (e CodedError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Msg)
}

func NewError(id *Id, code int, msg string) *CodedError {
	return &CodedError{id, code, msg}
}

func (r *Request) UnmarshalJSON(data []byte) error {
	panic("You can't unmarshal a request")
}

func (r *Response) MarshalJSON() ([]byte, error) {
	type Alias Response
	return json.Marshal(&struct {
		Version string `json:"jsonrpc"`
		*Alias
	}{
		Version: specVersion,
		Alias:   (*Alias)(r),
	})
}

func (r *Response) UnmarshalJSON(data []byte) error {
	type Alias Response
	raw := &struct {
		Version string `json:"jsonrpc"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}
	if r.Error == nil && r.Result == nil {
		return errors.New("Must send either a result or an error in a response")
	}
	return nil
	// note that I can't really do anything wrt
	// the Result type at this stage, because
	// I no longer have access to the Method,
	// which contains the Result's type info
}

func (r *RawResponse) MarshalJSON() ([]byte, error) {
	type Alias RawResponse
	return json.Marshal(&struct {
		Version string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result,omitempty"`
		*Alias
	}{
		Version: specVersion,
		Result:  r.Raw,
		Alias:   (*Alias)(r),
	})
}

func (r *RawResponse) UnmarshalJSON(data []byte) error {
	type Alias RawResponse
	raw := &struct {
		Version string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}
	// map these together
	r.Raw = raw.Result

	if len(r.Raw) == 0 && r.Error == nil {
		return errors.New("Must send either a result or an error in a response")
	}
	return nil
}

func GetParams(target Method) []interface{} {
	params := make([]interface{}, 0)
	v := reflect.ValueOf(target)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	typeOf := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fType := typeOf.Field(i)
		if !field.CanInterface() {
			continue
		}
		tag, _ := fType.Tag.Lookup("json")
		if _, omit := parseTag(tag); omit && isZero(field.Interface()) {
			continue
		}
		params = append(params, field.Interface())
	}
	return params
}

func parseTag(tag string) (name string, omitempty bool) {
	omitempty = false
	name = ""
	if tag == "" || tag == "-" {
		return name, omitempty
	}
	for i, field := range strings.Split(tag, ",") {
		if field == "omitempty" {
			omitempty = true
		}
		if i == 0 && field != "omitempty" {
			name = field
		}
	}
	return name, omitempty
}

func GetNamedParams(target Method) map[string]interface{} {
	params := make(map[string]interface{})
	v := reflect.ValueOf(target)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	typeOf := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fType := typeOf.Field(i)
		if !field.CanInterface() {
			continue
		}
		// if field is empty and has an 'omitempty' tag, leave it out
		var name string
		tag, ok := fType.Tag.Lookup("json")
		if ok {
			var omit bool
			name, omit = parseTag(tag)
			if omit && isZero(field.Interface()) {
				continue
			}
			if name == "" {
				name = strings.ToLower(fType.Name)
			}
		} else {
			name = strings.ToLower(fType.Name)
		}
		params[name] = field.Interface()
	}
	return params
}

func isZero(x interface{}) bool {
	return reflect.DeepEqual(x, reflect.Zero(reflect.TypeOf(x)).Interface())
}

// Map passed in params to the fields on the method, in listed order
func ParseParamArray(target Method, params []interface{}) error {
	targetValue := reflect.Indirect(reflect.ValueOf(target))
	fieldCount := targetFieldCount(targetValue)
	if fieldCount < len(params) {
		return errors.New(fmt.Sprintf("Too many parameters. Expected %d, received %d. See `help %s` for expected usage", fieldCount, len(params), target.Name()))
	}
	for i := range params {
		// it's possible that there's a mismatch between
		// 'settable' fields on the target and the params
		// that we've received. for simplicity's sake,
		// if you don't put all of your param names at the top
		// of your object, well that's your problem.
		fVal := targetValue.Field(i)
		value := params[i]
		err := innerParse(targetValue, fVal, value)
		if err != nil {
			return err
		}
	}
	return nil
}

// We assume that all interfaceable fields are in the right place.
// This lets us ignore non-interfaceable fields though.
func targetFieldCount(fieldVal reflect.Value) int {
	count := 0
	for i := 0; i < fieldVal.NumField(); i++ {
		if fieldVal.Field(i).CanInterface() {
			count++
		}
	}
	return count
}

func ParseNamedParams(target Method, params map[string]interface{}) error {
	targetValue := reflect.Indirect(reflect.ValueOf(target))
	return innerParseNamed(targetValue, params)
}

func innerParseNamed(targetValue reflect.Value, params map[string]interface{}) error {
	tType := targetValue.Type()
	for key, value := range params {
		found := false
		for i := 0; i < targetValue.NumField(); i++ {
			fVal := targetValue.Field(i)
			if !fVal.CanSet() {
				continue
			}
			fT := tType.Field(i)
			// check for the json tag match, as well a simple
			// lower case name match
			tag, _ := fT.Tag.Lookup("json")
			if tag == key || key == strings.ToLower(fT.Name) {
				found = true
				err := innerParse(targetValue, fVal, value)
				if err != nil {
					return err
				}
				break
			}
		}
		if !found && len(os.Getenv("GOLIGHT_STRICT_MODE")) > 0 {
			return NewError(nil, InvalidParams, fmt.Sprintf("No exported field found %s.%s", targetValue.Type().Name(), key))
		}
	}
	return nil
}

func innerParse(targetValue reflect.Value, fVal reflect.Value, value interface{}) error {
	if !fVal.CanSet() {
		var name string
		if fVal.IsValid() {
			name = fVal.Type().Name()
		} else {
			name = "<unknown>"
		}

		return NewError(nil, InvalidParams, fmt.Sprintf("Field %s.%s isn't settable. Are you sure it's exported?", targetValue.Type().Name(), name))
	}
	v := reflect.ValueOf(value)
	if fVal.Kind() == v.Kind() &&
		fVal.Kind() != reflect.Map &&
		fVal.Kind() != reflect.Slice {
		fVal.Set(v)
		return nil
	}

	// json.RawMessage escape hatch
	var eg json.RawMessage
	if fVal.Type() == reflect.TypeOf(eg) {
		out, err := json.Marshal(value)
		if err != nil {
			return err
		}
		jm := json.RawMessage(out)
		fVal.Set(reflect.ValueOf(jm))
		return nil
	}

	switch fVal.Kind() {
	case reflect.Map:
		fVal.Set(reflect.MakeMap(fVal.Type()))
		// the only types of maps that we can get thru the json
		// parser are map[string]interface{} ones
		mapVal := value.(map[string]interface{})
		keyType := fVal.Type().Key()
		for key, entry := range mapVal {
			eV := reflect.New(fVal.Type().Elem()).Elem()
			kV := reflect.ValueOf(key).Convert(keyType)
			err := innerParse(targetValue, eV, entry)
			if err != nil {
				return err
			}
			fVal.SetMapIndex(kV, eV)
		}
		return nil
	case reflect.Slice:
		// string -> []byte parsing
		sv, sok := value.(string)
		var xx []uint8
		if sok && fVal.Type() == reflect.TypeOf(xx) {
			// actually, we assume the string is
			// a hexstring becasuse ... yikes.
			// fixme: better would be to have a 'hexstring' type
			av, err := hex.DecodeString(sv)
			if err != nil {
				return err
			}
			fVal.Set(reflect.ValueOf(av))
			return nil
		}

		av := value.([]interface{})
		fVal.Set(reflect.MakeSlice(fVal.Type(), len(av), len(av)))
		for i := range av {
			err := innerParse(targetValue, fVal.Index(i), av[i])
			if err != nil {
				return err
			}
		}
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64:
		// float32 won't happen because of the json parser we're using
		if v.Type().Kind() != reflect.Float64 {
			return NewError(nil, InvalidParams, fmt.Sprintf("Expecting float64 input for %s.%s, but got %s", targetValue.Type().Name(), fVal.Type().Name(), v.Type()))
		}
		// Since our json parser (encoding/json) automatically defaults any
		// 'number' field to a float64, here we mangle mash it back into
		// an int field, since that's what we ostensibly wanted.
		//
		// there's probably a nicer way to do this but
		// the json/encoding lib checks for 'fitablilty' while
		// decoding so we don't have to worry about
		// an overflow here :D
		switch fVal.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
			reflect.Int64:
			fVal.SetInt(int64(value.(float64)))
			return nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16,
			reflect.Uint32, reflect.Uint64:
			fVal.SetUint(uint64(value.(float64)))
			return nil
		}
	case reflect.Ptr:
		if v.Kind() == reflect.Invalid {
			// i'm afraid that's a nil, my dear
			return nil
		}
		if v.Kind() != reflect.Map {
			return NewError(nil, InvalidParams, fmt.Sprintf("Types don't match. Expected a map[string]interface{} from the JSON, instead got %s", v.Kind().String()))
		}
		if fVal.IsNil() {
			// You need a new pointer object thing here
			// so allocate one with this voodoo-magique
			fVal.Set(reflect.New(fVal.Type().Elem()))
		}
		return innerParseNamed(fVal.Elem(), value.(map[string]interface{}))
	case reflect.Struct:
		if v.Kind() != reflect.Map {
			return NewError(nil, InvalidParams, fmt.Sprintf("Types don't match. Expected a map[string]interface{} from the JSON, instead got %s", v.Kind().String()))
		}
		return innerParseNamed(fVal, value.(map[string]interface{}))
	}
	return NewError(nil, InvalidParams, fmt.Sprintf("Incompatible types: %s.%s (%s) != %s", targetValue.Type().Name(), fVal.Type().Name(), fVal.Kind(), v.Type().Kind()))
}
