package rpcclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/pkg/errors"

	"crypto/tls"
	types "github.com/tendermint/tendermint/rpc/lib/types"
	"time"
)

// HTTPClient is a common interface for JSONRPCClient and URIClient.
type HTTPClient interface {
	Call(method string, params map[string]interface{}, result interface{}) (interface{}, error)
}

// TODO: Deprecate support for IP:PORT or /path/to/socket
func makeHTTPDialer(remoteAddr string) (string, func(string, string) (net.Conn, error)) {
	parts := strings.SplitN(remoteAddr, "://", 2)
	var protocol, address string
	if len(parts) == 1 {
		// default to tcp if nothing specified
		protocol, address = "tcp", remoteAddr
	} else if len(parts) == 2 {
		protocol, address = parts[0], parts[1]
	} else {
		// return a invalid message
		msg := fmt.Sprintf("Invalid addr: %s", remoteAddr)
		return msg, func(_ string, _ string) (net.Conn, error) {
			return nil, errors.New(msg)
		}
	}
	// accept http as an alias for tcp
	if protocol == "http" {
		protocol = "tcp"
	}

	// replace / with . for http requests (dummy domain)
	trimmedAddress := strings.Replace(address, "/", ".", -1)
	return trimmedAddress, func(proto, addr string) (net.Conn, error) {
		return net.Dial(protocol, address)
	}
}

func makeOtURIClientDialer(dialTimeout time.Duration) func(string, string) (net.Conn, error) {
	return func(netw, remoteUrl string) (net.Conn, error) {
		parts := strings.SplitN(remoteUrl, "://", 2)
		var protocol, address string
		if len(parts) == 1 {
			// default to tcp if nothing specified
			protocol, address = "tcp", remoteUrl
		} else if len(parts) == 2 {
			protocol, address = parts[0], parts[1]
		} else {
			// return a invalid message
			return nil, fmt.Errorf("Invalid addr: %s", remoteUrl)
		}
		// accept http as an alias for tcp
		if protocol == "http" {
			protocol = "tcp"
		}
		ipPort := strings.Split(address, "/")
		if len(ipPort) > 0 {
			address = ipPort[0]
		}
		c, err := net.DialTimeout(protocol, address, dialTimeout)
		if err != nil {
			return nil, fmt.Errorf("DialTimeout:%v, url=%s", err, address)
		}
		return c, nil
	}
}

// We overwrite the http.Client.Dial so we can do http over tcp or unix.
// remoteAddr should be fully featured (eg. with tcp:// or unix://)
func makeHTTPClient(remoteAddr string) (string, *http.Client) {
	address, dialer := makeHTTPDialer(remoteAddr)
	return "http://" + address, &http.Client{
		Transport: &http.Transport{
			Dial: dialer,
		},
	}
}

//------------------------------------------------------------------------------------

// JSONRPCClient takes params as a slice
type JSONRPCClient struct {
	address string
	client  *http.Client
}

// NewJSONRPCClient returns a JSONRPCClient pointed at the given address.
func NewJSONRPCClient(remote string) *JSONRPCClient {
	address, client := makeHTTPClient(remote)
	return &JSONRPCClient{
		address: address,
		client:  client,
	}
}

func (c *JSONRPCClient) Call(method string, params map[string]interface{}, result interface{}) (interface{}, error) {
	request, err := types.MapToRequest("jsonrpc-client", method, params)
	if err != nil {
		return nil, err
	}
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	// log.Info(string(requestBytes))
	requestBuf := bytes.NewBuffer(requestBytes)
	// log.Info(Fmt("RPC request to %v (%v): %v", c.remote, method, string(requestBytes)))
	httpResponse, err := c.client.Post(c.address, "text/json", requestBuf)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close() // nolint: errcheck

	responseBytes, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}
	// 	log.Info(Fmt("RPC response: %v", string(responseBytes)))
	return unmarshalResponseBytes(responseBytes, result)
}

//-------------------------------------------------------------

// URI takes params as a map
type URIClient struct {
	address string
	client  *http.Client
}

func NewURIClient(remote string) *URIClient {
	address, client := makeHTTPClient(remote)
	return &URIClient{
		address: address,
		client:  client,
	}
}

func (c *URIClient) Call(method string, params map[string]interface{}, result interface{}) (interface{}, error) {
	values, err := argsToURLValues(params)
	if err != nil {
		return nil, err
	}
	// log.Info(Fmt("URI request to %v (%v): %v", c.address, method, values))
	resp, err := c.client.PostForm(c.address+"/"+method, values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint: errcheck

	responseBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return unmarshalResponseBytes(responseBytes, result)
}

//------------------------------------------------

func unmarshalResponseBytes(responseBytes []byte, result interface{}) (interface{}, error) {
	// read response
	// if rpc/core/types is imported, the result will unmarshal
	// into the correct type
	// log.Notice("response", "response", string(responseBytes))
	var err error
	response := &types.RPCResponse{}
	err = json.Unmarshal(responseBytes, response)
	if err != nil {
		return nil, errors.Errorf("Error unmarshalling rpc response: %v", err)
	}
	if response.Error != nil {
		return nil, errors.Errorf("Response error: %v", response.Error)
	}
	// unmarshal the RawMessage into the result
	err = json.Unmarshal(response.Result, result)
	if err != nil {
		return nil, errors.Errorf("Error unmarshalling rpc response result: %v", err)
	}
	return result, nil
}

func argsToURLValues(args map[string]interface{}) (url.Values, error) {
	values := make(url.Values)
	if len(args) == 0 {
		return values, nil
	}
	err := argsToJson(args)
	if err != nil {
		return nil, err
	}
	for key, val := range args {
		values.Set(key, val.(string))
	}
	return values, nil
}

func argsToJson(args map[string]interface{}) error {
	for k, v := range args {
		rt := reflect.TypeOf(v)
		isByteSlice := rt.Kind() == reflect.Slice && rt.Elem().Kind() == reflect.Uint8
		if isByteSlice {
			bytes := reflect.ValueOf(v).Bytes()
			args[k] = fmt.Sprintf("0x%X", bytes)
			continue
		}

		// Pass everything else to go-wire
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		args[k] = string(data)
	}
	return nil
}

type OtURIClient struct {
	client *http.Client
}

type OtURIClientConf struct {
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	DisableKeepAlives     bool
	DisableCompression    bool
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	IdleConnTimeout       time.Duration
	TLSClientConfig       *tls.Config
}

func defaultOtURIClientConf() *OtURIClientConf {
	return &OtURIClientConf{
		DialTimeout:           20 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute,
		DisableKeepAlives:     false,
		DisableCompression:    true,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       2 * time.Minute,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
}

func NewOtURIClient(conf *OtURIClientConf) *OtURIClient {
	if conf == nil {
		conf = defaultOtURIClientConf()
	}
	dialer := makeOtURIClientDialer(conf.DialTimeout)
	return &OtURIClient{
		client: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: conf.ResponseHeaderTimeout,
				DisableCompression:    conf.DisableCompression,
				DisableKeepAlives:     conf.DisableKeepAlives,
				MaxIdleConns:          conf.MaxIdleConns,
				MaxIdleConnsPerHost:   conf.MaxIdleConnsPerHost,
				IdleConnTimeout:       conf.IdleConnTimeout,
				TLSClientConfig:       conf.TLSClientConfig,
				Dial:                  dialer,
			},
		},
	}
}

func (c *OtURIClient) trimAddress(remoteAddr string) string {
	parts := strings.SplitN(remoteAddr, "://", 2)
	var protocol, address string
	if len(parts) == 1 {
		// default to tcp if nothing specified
		protocol, address = "tcp", remoteAddr
	} else if len(parts) == 2 {
		protocol, address = parts[0], parts[1]
	} else {
		return parts[1]
	}
	// accept http as an alias for tcp
	if protocol == "http" {
		protocol = "tcp"
	}

	// replace / with . for http requests (dummy domain)
	trimmedAddress := strings.Replace(address, "/", ".", -1)
	return trimmedAddress
}

func (c *OtURIClient) buildUrl(remoteAddr, method string) string {
	trimeAddr := c.trimAddress(remoteAddr)
	return fmt.Sprintf("http://%s/%s", trimeAddr, method)
}

func (c *OtURIClient) Call(address string, method string, params map[string]interface{}, result interface{}) (interface{}, error) {
	values, err := argsToURLValues(params)
	if err != nil {
		return nil, err
	}
	url := c.buildUrl(address, method)
	resp, err := c.client.PostForm(url, values)
	if err != nil {
		return nil, fmt.Errorf("PostForm: %v", err)
	}
	defer resp.Body.Close()

	responseBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return unmarshalResponseBytes(responseBytes, result)
}

func (c *OtURIClient) CallWithJson(address string, method string, params map[string]interface{}, result interface{}) (interface{}, error) {
	request, err := types.MapToRequest("jsonrpc-client", method, params)
	if err != nil {
		return nil, err
	}
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	// log.Info(string(requestBytes))
	requestBuf := bytes.NewBuffer(requestBytes)
	//url := c.buildUrl(address, method)
	// log.Info(Fmt("RPC request to %v (%v): %v", c.remote, method, string(requestBytes)))
	httpResponse, err := c.client.Post("tcp://"+address, "text/json", requestBuf)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close() // nolint: errcheck

	responseBytes, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}
	// 	log.Info(Fmt("RPC response: %v", string(responseBytes)))
	return unmarshalResponseBytes(responseBytes, result)
}
