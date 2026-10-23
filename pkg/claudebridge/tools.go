// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/anthropics/anthropic-sdk-go"

	"unheaded/pkg/zhentools"
)

// NamedProvider is a tool source with a name for audit and routing
// ("champion", "mcp:zhen").
type NamedProvider struct {
	Name     string
	Provider zhentools.Provider
}

// toolNameRE is the Messages API's custom tool name constraint.
var toolNameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type owner struct {
	provider string
	p        zhentools.Provider
}

// toolset is the fixed tool list for a Bridge. It is built once, at
// construction: changing tools mid-conversation would invalidate the
// prompt cache and, on Claude Opus 5.5, earlier thinking blocks.
type toolset struct {
	owners map[string]owner
	params []anthropic.BetaToolUnionParam
	names  []string
}

func buildToolset(ctx context.Context, providers []NamedProvider) (*toolset, error) {
	ts := &toolset{owners: map[string]owner{}}
	for _, np := range providers {
		if np.Provider == nil {
			return nil, fmt.Errorf("provider %q is nil", np.Name)
		}
		tools, err := np.Provider.Tools(ctx)
		if err != nil {
			return nil, fmt.Errorf("provider %q: list tools: %w", np.Name, err)
		}
		for _, t := range tools {
			if !toolNameRE.MatchString(t.Name) {
				return nil, fmt.Errorf("provider %q: tool name %q is not valid for the Messages API", np.Name, t.Name)
			}
			if prev, dup := ts.owners[t.Name]; dup {
				return nil, fmt.Errorf("tool %q is offered by both %q and %q; refusing to guess which runs", t.Name, prev.provider, np.Name)
			}
			schema, err := schemaParam(t.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("provider %q: tool %q: %w", np.Name, t.Name, err)
			}
			tp := anthropic.BetaToolParam{Name: t.Name, InputSchema: schema}
			if t.Description != "" {
				tp.Description = anthropic.String(t.Description)
			}
			ts.owners[t.Name] = owner{provider: np.Name, p: np.Provider}
			ts.params = append(ts.params, anthropic.BetaToolUnionParam{OfTool: &tp})
			ts.names = append(ts.names, t.Name)
		}
	}
	return ts, nil
}

// schemaParam turns a provider's JSON Schema object into the SDK's input
// schema, keeping every keyword (additionalProperties, $defs, ...).
func schemaParam(raw json.RawMessage) (anthropic.BetaToolInputSchemaParam, error) {
	var out anthropic.BetaToolInputSchemaParam
	if len(raw) == 0 {
		return out, nil // {"type":"object"}: no arguments
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return out, fmt.Errorf("input schema is not a JSON object")
	}
	if typ, ok := m["type"]; ok && typ != "object" {
		return out, fmt.Errorf("input schema type is %v, want object", typ)
	}
	delete(m, "type")
	if props, ok := m["properties"]; ok {
		out.Properties = props
		delete(m, "properties")
	}
	if req, ok := m["required"]; ok {
		list, ok := req.([]any)
		if !ok {
			return out, fmt.Errorf("input schema required is not a list")
		}
		for _, r := range list {
			s, ok := r.(string)
			if !ok {
				return out, fmt.Errorf("input schema required holds a non-string")
			}
			out.Required = append(out.Required, s)
		}
		delete(m, "required")
	}
	if len(m) > 0 {
		out.ExtraFields = m
	}
	return out, nil
}
