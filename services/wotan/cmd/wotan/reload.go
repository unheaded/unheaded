// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package main

import (
	"context"
	"os"

	"github.com/rs/zerolog/log"
	"unheaded/services/wotan/internal/api"
)

// reloadTopicConfigOnHUP reloads the topic config from path on every
// signal from hup (SIGHUP: `systemctl kill -s HUP unheaded-wotan`) until
// ctx ends. Only the allowlists change; a refused reload is logged and
// counted and the running config stays as it was (S-TRUE-WIRE Step 155).
func reloadTopicConfigOnHUP(ctx context.Context, hup <-chan os.Signal, cfg *api.TopicConfig, path string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-hup:
			res, err := cfg.Reload(path)
			if err != nil {
				log.Error().Err(err).Str("config_path", path).Msg("topic_config_reload_refused")
				continue
			}
			log.Info().
				Str("config_path", path).
				Int("added", res.Added).
				Int("removed", res.Removed).
				Msg("topic_config_reloaded")
		}
	}
}
