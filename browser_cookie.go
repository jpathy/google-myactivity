package myactivity

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"syscall"
	"time"

	"github.com/cenkalti/backoff"
	multierror "github.com/hashicorp/go-multierror"
	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/dom"
	"github.com/mafredri/cdp/protocol/network"
	"github.com/mafredri/cdp/protocol/page"
	"github.com/mafredri/cdp/protocol/runtime"
	"github.com/mafredri/cdp/rpcc"
)

type key int

const (
	cmdKey        key = 0
	clientKey     key = 1
	portKey       key = 2
	myActivityURL     = "https://myactivity.google.com"
)

func runChrome(ctx context.Context, execPath, userDataDir, port string) (rctx context.Context, err error) {
	rctx = ctx

	execArgs := []string{
		"--user-data-dir=" + userDataDir,
		"--remote-debugging-port=" + port,
		// "--headless", // TODO: bug in browser
		"--disable-gpu",
		"--hide-scrollbars",
		"--no-first-run",
		"--no-default-browser-check",
		"--mute-audio",
		"--disable-sync",
		"--disable-extensions",
		"--disable-prompt-on-repost",
		"about:blank",
	}

	cmd := exec.CommandContext(ctx, execPath, execArgs...)
	if err = cmd.Start(); err != nil {
		return
	}

	bkoff := backoff.NewExponentialBackOff()
	bkoff.InitialInterval = 10 * time.Millisecond
	bkoff.MaxInterval = 10 * time.Second
	bkoff.MaxElapsedTime = time.Minute
	dt := devtool.New("http://127.0.0.1:" + port)
	var pg *devtool.Target
	if err = backoff.Retry(func() error {
		var lerr error
		pg, lerr = dt.Get(ctx, devtool.Page)
		return lerr
	}, backoff.WithContext(bkoff, ctx)); err != nil {
		return
	}

	conn, err := rpcc.Dial(pg.WebSocketDebuggerURL)
	if err != nil {
		return
	}
	cl := cdp.NewClient(conn)

	rctx = context.WithValue(rctx, cmdKey, cmd)
	rctx = context.WithValue(rctx, clientKey, cl)
	rctx = context.WithValue(rctx, portKey, port)
	return
}

func stopChrome(ctx context.Context) error {
	defer func() {
		if cmd, ok := ctx.Value(cmdKey).(*exec.Cmd); !ok {
			log.Fatalln("Implementation Bug!!")
		} else {
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				log.Print(err)
			}
		}
	}()

	var (
		port string
		ok   bool
	)
	if port, ok = ctx.Value(portKey).(string); !ok {
		log.Fatalln("Implementation Bug")
	}
	dt := devtool.New("http://127.0.0.1:" + port)

	tgts, err := dt.List(ctx)
	if err != nil {
		return err
	}

	gcount := len(tgts)
	errC := make(chan error, gcount)
	for _, t := range tgts {
		if t.Type == devtool.Page {
			go func(t *devtool.Target, errC chan<- error) {
				errC <- dt.Close(ctx, t)
			}(t, errC)
		}
	}

	for i := 0; i < gcount; i++ {
		if lerr := <-errC; lerr != nil {
			err = multierror.Append(err, lerr)
		}
	}

	return err
}

// browserCookieAndSig extracts the cookies and signature for logged in chrome session to myactivity,
// which we will pass in the http headers while making api requests.
func browserCookieAndSig(ctx context.Context, execPath, userDataDir, port string) (cookies string, sig string, err error) {
	if ctx, err = runChrome(ctx, execPath, userDataDir, port); err != nil {
		return
	}
	defer func() {
		if lerr := stopChrome(ctx); lerr != nil {
			log.Printf("Failure during stopChrome : %v", lerr)
		}
	}()

	var (
		cl *cdp.Client
		ok bool
	)
	if cl, ok = ctx.Value(clientKey).(*cdp.Client); !ok {
		log.Fatalln("Implementation Bug!!")
	}

	// Enable necessary events
	if err = cl.Page.Enable(ctx); err != nil {
		return
	}

	// navigate URL with timeout.
	if err = navigate(ctx, cl.Page, myActivityURL); err != nil {
		return
	}

	// get root node
	doc, err := cl.DOM.GetDocument(ctx, nil)
	if err != nil {
		return
	}

	// Find the search/filter box on myactivity, this tests for login
	searchBox, err := cl.DOM.QuerySelectorAll(ctx,
		dom.NewQuerySelectorAllArgs(doc.Root.NodeID, `#main-content > div.main-column-width > div.fp-search-privacy-holder > md-card`))
	if err != nil {
		return
	} else if len(searchBox.NodeIDs) == 0 {
		return "", "", fmt.Errorf("Current user profile is not logged in for %s", myActivityURL)
	}

	// get cookies
	cks, err := cl.Network.GetCookies(ctx, network.NewGetCookiesArgs().SetURLs([]string{"https://google.com"}))
	if err != nil {
		return
	}

	properties, err := cl.Runtime.Evaluate(ctx, runtime.NewEvaluateArgs(`window.HISTORY_xsrf`))
	if err != nil {
		return
	} else if err = json.Unmarshal(properties.Result.Value, &sig); err != nil {
		return "", "", fmt.Errorf("Invalid signature for %s : %v", myActivityURL, err)
	}

	for i, cookie := range cks.Cookies {
		e := cookie.Name + "=" + cookie.Value
		if i == len(cks.Cookies)-1 {
			cookies += e
		} else {
			cookies += e + "; "
		}
	}

	return
}

func navigate(ctx context.Context, pageClient cdp.Page, url string) error {
	// Make sure Page events are enabled.
	err := pageClient.Enable(ctx)
	if err != nil {
		return err
	}

	// Open client for DOMContentEventFired to block until DOM has fully loaded.
	domContentEventFired, err := pageClient.DOMContentEventFired(ctx)
	if err != nil {
		return err
	}
	defer domContentEventFired.Close()

	_, err = pageClient.Navigate(ctx, page.NewNavigateArgs(url))
	if err != nil {
		return err
	}

	_, err = domContentEventFired.Recv()
	return err
}
