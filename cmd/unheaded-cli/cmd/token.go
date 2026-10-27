// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package cmd

import (
	"errors"
	"flag"
	"fmt"

	"unheaded/pkg/auth"
)

// NewTokenCommand creates `token`: print one EdDSA service token from the
// CLI's own key (sub unheaded-cli) for a service, for shell calls that
// cannot mint (ADR-103 §2):
//
//	curl -H "Authorization: Bearer $(unheaded token --aud wotan)" http://localhost:18000/api/v1/topics
//
// The token lives 5 minutes and is good only at the service it names.
// The key is the one every CLI call uses (AUTH_KEY_FILE, then
// ~/.config/unheaded/cli.key, then /etc/unheaded/keys/unheaded-cli.key);
// UNHEADED_TOKEN is not printed. No key or an unusable one is an error,
// never an empty token.
func NewTokenCommand() *Command {
	var aud string
	flags := flag.NewFlagSet("token", flag.ContinueOnError)
	flags.StringVar(&aud, "aud", "", "The service the token is for (its auth identity, e.g. wotan, unheaded-daemon)")

	return &Command{
		Name:  "token",
		Short: "Print a 5-minute service token for one service (for curl)",
		Usage: "unheaded token --aud <service>",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected argument %q", args[0])
			}
			if !auth.ValidKeyName(aud) {
				return fmt.Errorf("--aud %q: want a service identity ([a-z0-9][a-z0-9_-]*)", aud)
			}
			keyFile := cliKeyFile()
			if keyFile == "" {
				return errors.New("no CLI key: set AUTH_KEY_FILE, or create one with unheaded-keygen unheaded-cli (ADR-103)")
			}
			m, err := auth.LoadServiceTokenMinter(cliKeyName, keyFile)
			if err != nil {
				return fmt.Errorf("CLI key %s: %w", keyFile, err)
			}
			tok, err := m.Mint(aud)
			if err != nil {
				return err
			}
			return ctx.Output.WriteStringln(tok)
		},
	}
}
