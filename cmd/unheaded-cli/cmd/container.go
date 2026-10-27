// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cmd provides container commands for THE GAUNTLETS CLI.
package cmd

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"unheaded/cmd/unheaded-cli/output"
)

// cuirassSpec and cuirassState mirror Cuirass's ContainerSpec and
// ContainerState (cmd/unheaded-daemon).
type cuirassSpec struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	CPU    int               `json:"cpu"`
	Memory int64             `json:"memory"`
	Labels map[string]string `json:"labels,omitempty"`
}

type cuirassState struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	IP      string `json:"ip"`
	Health  string `json:"health"`
	Managed bool   `json:"managed"`
}

// ContainerInfo is one container as Cuirass reports it: what is desired
// and, if LXD has it, what is there.
type ContainerInfo struct {
	ID      string        `json:"id"`
	Desired *cuirassSpec  `json:"desired"`
	Actual  *cuirassState `json:"actual"`
}

// ContainerListResult represents the output for container list.
type ContainerListResult struct {
	Containers []ContainerInfo `json:"containers"`
}

func (c ContainerInfo) status() string {
	if c.Actual == nil {
		return "missing"
	}
	return c.Actual.Status
}

// WriteTable implements output.TableWriter.
func (r *ContainerListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("NAME", "STATUS", "IMAGE", "IP", "MANAGED", "CPU", "MEMORY")
	} else {
		t.SetHeaders("NAME", "STATUS", "IMAGE", "IP")
	}
	for _, c := range r.Containers {
		status := output.StatusOutput{Status: c.status()}.Colorize(t.Color)
		image, ip, managed, cpu, mem := "-", "-", "-", "-", "-"
		if c.Desired != nil {
			image = c.Desired.Image
			if c.Desired.CPU > 0 {
				cpu = strconv.Itoa(c.Desired.CPU)
			}
			if c.Desired.Memory > 0 {
				mem = formatBytes(c.Desired.Memory)
			}
		}
		if c.Actual != nil {
			if c.Actual.IP != "" {
				ip = c.Actual.IP
			}
			managed = strconv.FormatBool(c.Actual.Managed)
		}
		if wide {
			t.AddRow(c.ID, status, image, ip, managed, cpu, mem)
		} else {
			t.AddRow(c.ID, status, image, ip)
		}
	}
}

// NewContainerCommand creates the container command.
func NewContainerCommand() *Command {
	cmd := &Command{
		Name:  "container",
		Short: "Manage containers",
		Long: `Desired container state, through Cuirass (the context's endpoint).

Cuirass reconciles LXD to the desired state: create and delete change what
is desired, and Cuirass acts on it only with CUIRASS_ENFORCE=true (deletes
also need CUIRASS_PRUNE=true). Writes need AUTH_ENABLED on Cuirass and a
token in UNHEADED_TOKEN. Start, stop and exec fail: Cuirass has none; use lxc.`,
		Aliases:     []string{"ct", "containers"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newContainerListCommand())
	cmd.AddCommand(newContainerCreateCommand())
	cmd.AddCommand(newContainerDeleteCommand())
	cmd.AddCommand(newContainerInspectCommand())
	for _, c := range []struct {
		name, short, usage string
		aliases            []string
	}{
		{"start", "Start a container", "unheaded container start <name>", []string{"up"}},
		{"stop", "Stop a container", "unheaded container stop <name>", []string{"down"}},
		{"exec", "Execute a command in a container", "unheaded container exec <name> -- <command>", nil},
	} {
		cmd.AddCommand(noAPICommand("container "+c.name, c.short, c.usage,
			"Cuirass reconciles desired state and has no "+c.name+"; use lxc "+c.name, c.aliases...))
	}

	return cmd
}

func cuirassURL(ctx *Context, path string) (string, error) {
	c, err := ctx.cluster()
	if err != nil {
		return "", err
	}
	return serviceURL(c.Endpoint, path)
}

// cuirassErr adds where the CLI looked when Cuirass does not answer at
// all: compose publishes it on 19006, the deb on 17000 (the default).
func cuirassErr(ctx *Context, what string, err error) error {
	var ae *apiError
	if errors.As(err, &ae) {
		return fmt.Errorf("%s: %w", what, err)
	}
	endpoint := "?"
	if c, cerr := ctx.cluster(); cerr == nil {
		endpoint = c.Endpoint
	}
	return fmt.Errorf("%s: %w (Cuirass endpoint %s; change it with: unheaded config set contexts.<context>.endpoint http://host:19006)", what, err, endpoint)
}

func containerPath(name string) string {
	return "/api/v1/containers/" + url.PathEscape(name)
}

func newContainerListCommand() *Command {
	var filter string

	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.StringVar(&filter, "filter", "", "Only this status (running, stopped, missing)")
	flags.StringVar(&filter, "f", "", "Filter by status (short)")

	return &Command{
		Name:    "list",
		Short:   "List desired containers and their actual state",
		Usage:   "unheaded container list [flags]",
		Aliases: []string{"ls"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			target, err := cuirassURL(ctx, "/api/v1/containers")
			if err != nil {
				return err
			}
			var list []ContainerInfo
			if err := callCuirass(http.MethodGet, target, nil, &list); err != nil {
				return cuirassErr(ctx, "list containers", err)
			}
			sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
			result := &ContainerListResult{Containers: []ContainerInfo{}}
			for _, c := range list {
				if filter != "" && !strings.EqualFold(c.status(), filter) {
					continue
				}
				result.Containers = append(result.Containers, c)
			}
			return ctx.Output.Write(result)
		},
	}
}

func newContainerCreateCommand() *Command {
	var (
		image  string
		cpu    int
		memory string
		labels string
	)

	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	flags.StringVar(&image, "image", "", "Image: a local alias, images:<alias> or ubuntu:<alias>")
	flags.StringVar(&image, "i", "", "Image (short)")
	flags.IntVar(&cpu, "cpu", 0, "CPU cores (0 = LXD default)")
	flags.StringVar(&memory, "memory", "", "Memory limit, e.g. 512MiB, 2GiB (empty = LXD default)")
	flags.StringVar(&labels, "labels", "", "Labels (key=value,...)")
	flags.StringVar(&labels, "l", "", "Labels (short)")

	return &Command{
		Name:  "create",
		Short: "Add a container to the desired state",
		Usage: "unheaded container create <name> --image <image> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("exactly one container name required")
			}
			if image == "" {
				return fmt.Errorf("--image is required")
			}
			name := args[0]
			spec := cuirassSpec{ID: name, Name: name, Image: image, CPU: cpu}
			if memory != "" {
				n, err := parseBytes(memory)
				if err != nil {
					return err
				}
				spec.Memory = n
			}
			if labels != "" {
				spec.Labels = map[string]string{}
				for _, pair := range strings.Split(labels, ",") {
					k, v, ok := strings.Cut(pair, "=")
					if !ok || k == "" {
						return fmt.Errorf("invalid label %q (want key=value)", pair)
					}
					spec.Labels[k] = v
				}
			}
			target, err := cuirassURL(ctx, containerPath(name))
			if err != nil {
				return err
			}
			if err := callCuirass(http.MethodPut, target, spec, nil); err != nil {
				return cuirassErr(ctx, "create "+name, err)
			}
			ctx.Output.WriteSuccess("Container '%s' added to the desired state; Cuirass reconciles it", name)
			return nil
		},
	}
}

func newContainerDeleteCommand() *Command {
	return &Command{
		Name:    "delete",
		Short:   "Remove containers from the desired state",
		Usage:   "unheaded container delete <name>...",
		Aliases: []string{"rm", "remove"},
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("at least one container name required")
			}
			failed := 0
			for _, name := range args {
				target, err := cuirassURL(ctx, containerPath(name))
				if err != nil {
					return err
				}
				if err := callCuirass(http.MethodDelete, target, nil, nil); err != nil {
					ctx.Output.WriteError("delete %s: %v", name, err)
					failed++
					continue
				}
				ctx.Output.WriteSuccess("Container '%s' removed from the desired state; Cuirass deletes it only when pruning", name)
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d deletes failed", failed, len(args))
			}
			return nil
		},
	}
}

func newContainerInspectCommand() *Command {
	return &Command{
		Name:    "inspect",
		Short:   "Show a container's desired and actual state",
		Usage:   "unheaded container inspect <name>",
		Aliases: []string{"info"},
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("exactly one container name required")
			}
			name := args[0]
			target, err := cuirassURL(ctx, containerPath(name))
			if err != nil {
				return err
			}
			var c ContainerInfo
			if err := callCuirass(http.MethodGet, target, nil, &c); err != nil {
				if isHTTPStatus(err, http.StatusNotFound) {
					return fmt.Errorf("container %s: not desired and not in LXD", name)
				}
				return cuirassErr(ctx, "inspect "+name, err)
			}
			if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
				return ctx.Output.Write(c)
			}

			w := ctx.Output
			_ = w.WriteStringln(output.Bold("Container: " + name))
			kv := output.NewKeyValueTable(w.Out()).SetColor(w.Color())
			kv.AddWithStatus("Status", c.status())
			if d := c.Desired; d != nil {
				kv.Add("Desired", "yes")
				kv.Add("Image", d.Image)
				if d.CPU > 0 {
					kv.Add("CPU", strconv.Itoa(d.CPU))
				}
				if d.Memory > 0 {
					kv.Add("Memory", formatBytes(d.Memory))
				}
				keys := make([]string, 0, len(d.Labels))
				for k := range d.Labels {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					kv.Add("Label "+k, d.Labels[k])
				}
			} else {
				kv.Add("Desired", "no")
			}
			if a := c.Actual; a != nil {
				kv.Add("IP", a.IP)
				kv.Add("Managed", strconv.FormatBool(a.Managed))
				kv.Add("Health", a.Health)
			}
			kv.Render()
			return nil
		},
	}
}

// parseBytes reads "512MiB", "2GiB", "1GB" or a plain byte count.
func parseBytes(s string) (int64, error) {
	units := []struct {
		suffix string
		mult   int64
	}{
		{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40},
		{"KB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"TB", 1e12}, {"B", 1},
	}
	mult := int64(1)
	num := s
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			num, mult = strings.TrimSuffix(s, u.suffix), u.mult
			break
		}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(num), 10, 64)
	if err != nil || n <= 0 || n > (1<<62)/mult {
		return 0, fmt.Errorf("invalid size %q (e.g. 512MiB, 2GiB)", s)
	}
	return n * mult, nil
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
