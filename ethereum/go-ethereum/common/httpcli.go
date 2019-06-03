package common

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

const (
	defaultDialTimeout = 10 * time.Second
	keepAliveInterval  = 30 * time.Second
)

var (
	gTransport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
		ResponseHeaderTimeout: 2 * time.Minute,
		DisableCompression:    true,
		DisableKeepAlives:     false,
		IdleConnTimeout:       2 * time.Minute,
		MaxIdleConns:          4,
		MaxIdleConnsPerHost:   2,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{Timeout: defaultDialTimeout, KeepAlive: keepAliveInterval}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	gHttpClient = &http.Client{
		Transport: gTransport,
	}
)

func HttpPost(url string, request interface{}) ([]byte, error) {
	client := gHttpClient
	data, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("NewRequest: err=%v", err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("nc", "pocketapi")
	req = req.WithContext(context.Background())
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("StatusCode %d, Resp %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func HttpPostWithHeader(url string, data []byte, header map[string]string) ([]byte, error) {
	client := gHttpClient
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("NewRequest: err=%v", err)
	}

	req.Header.Add("Content-Type", "application/json")
	for k, v := range header {
		req.Header.Add(k, v)
	}
	req = req.WithContext(context.Background())
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.Do: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("StatusCode %d, Resp %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func HttpGet(addr string) ([]byte, error) {
	resp, err := gHttpClient.Get(addr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("StatusCode %d, Resp %s", resp.StatusCode, string(data))
	}
	return data, nil
}
