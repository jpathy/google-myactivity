package myactivity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

const (
	defaultDebuggingPort = "9222"
)

// Client stores various configuration options.
type Client struct {
	ChromeExecPath string
	ChromeDataDir  string
	DebugPort      string
}

// ActivityDecoderFunc is the type of user provided activity decoder function.
type ActivityDecoderFunc func(json.RawMessage) (interface{}, error)

// NewClient creates a new activity manager client validating configuration options.
func NewClient(chromePath, chromeDataDir, debugPort string) *Client {
	if chromePath == "" {
		chromePath = findChromePath()
	}
	if _, err := strconv.Atoi(debugPort); err != nil || debugPort == "" {
		debugPort = defaultDebuggingPort
	}

	return &Client{
		ChromeExecPath: chromePath,
		ChromeDataDir:  chromeDataDir,
		DebugPort:      debugPort,
	}
}

// FetchActivities fetches activities with query params and sends results to the returned channel processed by decoderFn.
// On any error, the error channel is used to send the error. The error channel is non-blocking.
func (c *Client) FetchActivities(ctx context.Context, params url.Values, decoderFn ActivityDecoderFunc) (<-chan []interface{}, <-chan error) {
	u, err := url.Parse(myActivityURL)
	if err != nil {
		log.Fatalf("Implementation Bug : %v\n", err)
	}
	u.Path = "item"
	params.Set("jspb", "1")
	u.RawQuery = params.Encode()

	dataC := make(chan []interface{}, 16)
	errC := make(chan error, 1)
	go func() {
		var err error
		errFn := func(err error) {
			select {
			case <-ctx.Done():
			case errC <- err:
			}
		}
		defer func() {
			if err != nil {
				errFn(err)
			}
			close(errC)
			close(dataC)
		}()

		cookies, sig, err := browserCookieAndSig(ctx, c.ChromeExecPath, c.ChromeDataDir, c.DebugPort)
		if err != nil {
			return
		}
		header := http.Header{}
		header.Set("content-type", "application/json")
		header.Set("user-agent", "Mozilla/5.0")
		header.Set("cookie", cookies)

		var marker struct {
			Sig string `json:"sig"`
			CT  string `json:"ct,omitempty"`
		}
		marker.Sig = sig
		var reqb bytes.Buffer
		jsenc := json.NewEncoder(&reqb)

		prevct := "initialrandom"
		for marker.CT != prevct {
			if err = jsenc.Encode(marker); err != nil {
				return
			}
			var req *http.Request
			if req, err = http.NewRequest("POST", u.String(), &reqb); err != nil {
				return
			}
			req.Header = header
			req = req.WithContext(ctx)

			var (
				resp     *http.Response
				respData []interface{}
			)
			if resp, err = http.DefaultClient.Do(req); err != nil {
				return
			}

			prevct = marker.CT
			respData, marker.CT, err = decodeActivities(resp.Body, decoderFn)
			resp.Body.Close()

			select {
			case <-ctx.Done():
			case dataC <- respData:
			}
		}
	}()

	return dataC, errC
}

func decodeActivities(body io.Reader, decoderFn ActivityDecoderFunc) (results []interface{}, ct string, err error) {
	var b [6]byte
	if _, err = io.ReadFull(body, b[:]); err != nil || !bytes.Equal(b[:], []byte(")]}',\n")) {
		err = fmt.Errorf("Unexpected api response : %v", err)
		return
	}

	var (
		ii   []json.RawMessage
		vals []json.RawMessage
	)
	if err = json.NewDecoder(body).Decode(&ii); err != nil || len(ii) != 2 {
		err = fmt.Errorf("Failed to decode json response : %v", err)
		return
	}

	if err = json.Unmarshal(ii[1], &ct); err != nil {
		return
	}

	if ii[0] == nil {
		err = nil
		return
	} else if err = json.Unmarshal(ii[0], &vals); err != nil {
		return
	}

	for _, e := range vals {
		var res interface{}
		if res, err = decoderFn(e); err != nil {
			return
		}
		if res != nil {
			results = append(results, res)
		}
	}

	// reset err to nil
	err = nil
	return
}
