// SPDX-License-Identifier: GPL-3.0-or-later
//
// wsclient — minimal WebSocket client for exercising upc-tty-bridge end to
// end without a browser (S5 Phase 4, Steps 126-129). Lives under testdata/ so
// `go build ./...` skips it; build it by explicit path:
//
//	go build -o wsclient ./cmd/upc-tty-bridge/testdata/wsclient
//	wsclient -url ws://127.0.0.1:26100/console?instance=111 \
//	  -after 10s -send 'echo hello\n' -wait 30s
//
// Each -send is one WS message (\n and \t escapes are expanded), sent -gap
// apart after -after. Everything the bridge sends back is written to stdout
// with NUL bytes dropped.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type sends []string

func (s *sends) String() string     { return strings.Join(*s, ",") }
func (s *sends) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	url := flag.String("url", "ws://127.0.0.1:26100/console?instance=111", "bridge console URL")
	after := flag.Duration("after", 10*time.Second, "wait before the first send")
	gap := flag.Duration("gap", 5*time.Second, "wait between sends")
	wait := flag.Duration("wait", 30*time.Second, "total time to keep reading")
	var msgs sends
	flag.Var(&msgs, "send", "message to send (repeatable; \\n and \\t expanded)")
	flag.Parse()

	conn, resp, err := websocket.DefaultDialer.Dial(*url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial %s: %v\n", *url, err)
		os.Exit(1)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	defer conn.Close()

	go func() {
		time.Sleep(*after)
		for i, m := range msgs {
			if i > 0 {
				time.Sleep(*gap)
			}
			m = strings.NewReplacer(`\n`, "\n", `\t`, "\t").Replace(m)
			if err := conn.WriteMessage(websocket.TextMessage, []byte(m)); err != nil {
				fmt.Fprintf(os.Stderr, "send: %v\n", err)
				return
			}
			fmt.Fprintf(os.Stderr, "[wsclient] sent %d bytes\n", len(m))
		}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(*wait))
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		out := make([]byte, 0, len(data))
		for _, b := range data {
			if b != 0 {
				out = append(out, b)
			}
		}
		_, _ = os.Stdout.Write(out)
	}
}
