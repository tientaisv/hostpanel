package docker

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
	socketPath string
}

func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = "/var/run/docker.sock"
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.DialTimeout("unix", socketPath, 10*time.Second)
		},
	}

	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   0, // Allow continuous streaming
		},
	}
}

func (c *Client) Get(path string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", "http://localhost"+path, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func (c *Client) Post(path string, body []byte) ([]byte, int, error) {
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequest("POST", "http://localhost"+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	resBody, err := ioutil.ReadAll(resp.Body)
	return resBody, resp.StatusCode, err
}

func (c *Client) Delete(path string) ([]byte, int, error) {
	req, err := http.NewRequest("DELETE", "http://localhost"+path, nil)
	if err != nil {
		return nil, 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	resBody, err := ioutil.ReadAll(resp.Body)
	return resBody, resp.StatusCode, err
}

// RawConn opens a raw Unix socket connection for Hijack
func (c *Client) RawConn() (net.Conn, error) {
	return net.Dial("unix", c.socketPath)
}
