// Package cmd provides container commands for THE GAUNTLETS CLI.
package cmd

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"unheaded/cmd/unheaded-cli/output"
	"unheaded/pkg/lxd"
)

// ContainerListResult represents the output for container list.
type ContainerListResult struct {
	Containers []ContainerInfo `json:"containers"`
}

// ContainerInfo represents container information for output.
type ContainerInfo struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Image      string    `json:"image"`
	IPs        []string  `json:"ips"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	Profiles   []string  `json:"profiles"`
	Ephemeral  bool      `json:"ephemeral"`
}

// WriteTable implements output.TableWriter.
func (r *ContainerListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("NAME", "STATUS", "IMAGE", "IPS", "CREATED", "LAST USED", "PROFILES", "EPHEMERAL")
	} else {
		t.SetHeaders("NAME", "STATUS", "IMAGE", "IPS", "CREATED")
	}

	for _, c := range r.Containers {
		status := output.StatusOutput{Status: c.Status}.Colorize(t.Color)
		ips := strings.Join(c.IPs, ", ")
		if ips == "" {
			ips = "-"
		}
		created := formatAge(c.CreatedAt)

		if wide {
			lastUsed := formatAge(c.LastUsedAt)
			profiles := strings.Join(c.Profiles, ", ")
			ephemeral := "no"
			if c.Ephemeral {
				ephemeral = "yes"
			}
			t.AddRow(c.Name, status, c.Image, ips, created, lastUsed, profiles, ephemeral)
		} else {
			t.AddRow(c.Name, status, c.Image, ips, created)
		}
	}
}

// NewContainerCommand creates the container command.
func NewContainerCommand() *Command {
	cmd := &Command{
		Name:        "container",
		Short:       "Manage containers",
		Long:        "Commands for managing LXD containers in the Unheaded Kingdom.",
		Aliases:     []string{"ct", "containers"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newContainerListCommand())
	cmd.AddCommand(newContainerCreateCommand())
	cmd.AddCommand(newContainerStartCommand())
	cmd.AddCommand(newContainerStopCommand())
	cmd.AddCommand(newContainerDeleteCommand())
	cmd.AddCommand(newContainerExecCommand())
	cmd.AddCommand(newContainerInspectCommand())

	return cmd
}

func newContainerListCommand() *Command {
	var (
		all    bool
		filter string
	)

	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.BoolVar(&all, "all", false, "Show all containers (including stopped)")
	flags.BoolVar(&all, "a", false, "Show all containers (short)")
	flags.StringVar(&filter, "filter", "", "Filter by status (running, stopped, frozen)")
	flags.StringVar(&filter, "f", "", "Filter by status (short)")

	return &Command{
		Name:    "list",
		Short:   "List containers",
		Usage:   "unheaded container list [flags]",
		Aliases: []string{"ls"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			client, err := getLXDClient(ctx)
			if err != nil {
				return err
			}

			containers, err := client.ListContainers(context.Background())
			if err != nil {
				return fmt.Errorf("failed to list containers: %w", err)
			}

			result := &ContainerListResult{}
			for _, c := range containers {
				// Apply filters
				if !all && c.Status == "Stopped" {
					continue
				}
				if filter != "" && !strings.EqualFold(c.Status, filter) {
					continue
				}

				ips := extractIPs(c)
				info := ContainerInfo{
					Name:       c.Name,
					Status:     c.Status,
					Image:      getContainerImage(c),
					IPs:        ips,
					CreatedAt:  c.CreatedAt,
					LastUsedAt: c.LastUsedAt,
					Profiles:   c.Profiles,
					Ephemeral:  c.Ephemeral,
				}
				result.Containers = append(result.Containers, info)
			}

			return ctx.Output.Write(result)
		},
	}
}

func newContainerCreateCommand() *Command {
	var (
		image     string
		profiles  string
		ephemeral bool
		config    string
	)

	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	flags.StringVar(&image, "image", "nixos:latest", "Container image to use")
	flags.StringVar(&image, "i", "nixos:latest", "Container image (short)")
	flags.StringVar(&profiles, "profiles", "default", "Comma-separated list of profiles")
	flags.StringVar(&profiles, "p", "default", "Profiles (short)")
	flags.BoolVar(&ephemeral, "ephemeral", false, "Create ephemeral container")
	flags.BoolVar(&ephemeral, "e", false, "Ephemeral (short)")
	flags.StringVar(&config, "config", "", "Container config (key=value,...)")
	flags.StringVar(&config, "c", "", "Container config (short)")

	return &Command{
		Name:  "create",
		Short: "Create a new container",
		Usage: "unheaded container create <name> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("container name required")
			}
			name := args[0]

			client, err := getLXDClient(ctx)
			if err != nil {
				return err
			}

			cfg := lxd.ContainerConfig{
				Name:      name,
				Image:     image,
				Profiles:  strings.Split(profiles, ","),
				Ephemeral: ephemeral,
			}

			// Parse config
			if config != "" {
				cfg.Config = make(map[string]string)
				for _, pair := range strings.Split(config, ",") {
					parts := strings.SplitN(pair, "=", 2)
					if len(parts) == 2 {
						cfg.Config[parts[0]] = parts[1]
					}
				}
			}

			ctx.Logger.Info().Str("name", name).Str("image", image).Msg("creating container")

			op, err := client.CreateContainer(context.Background(), cfg)
			if err != nil {
				return fmt.Errorf("failed to create container: %w", err)
			}

			if err := client.WaitOperation(context.Background(), op); err != nil {
				return fmt.Errorf("create operation failed: %w", err)
			}

			ctx.Output.WriteSuccess("Container '%s' created successfully", name)
			return nil
		},
	}
}

func newContainerStartCommand() *Command {
	return &Command{
		Name:    "start",
		Short:   "Start a container",
		Usage:   "unheaded container start <name>...",
		Aliases: []string{"up"},
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("at least one container name required")
			}

			client, err := getLXDClient(ctx)
			if err != nil {
				return err
			}

			for _, name := range args {
				ctx.Logger.Info().Str("name", name).Msg("starting container")

				op, err := client.StartContainer(context.Background(), name)
				if err != nil {
					ctx.Output.WriteError("Failed to start container '%s': %v", name, err)
					continue
				}

				if err := client.WaitOperation(context.Background(), op); err != nil {
					ctx.Output.WriteError("Start operation failed for '%s': %v", name, err)
					continue
				}

				ctx.Output.WriteSuccess("Container '%s' started", name)
			}

			return nil
		},
	}
}

func newContainerStopCommand() *Command {
	var (
		force   bool
		timeout int
	)

	flags := flag.NewFlagSet("stop", flag.ContinueOnError)
	flags.BoolVar(&force, "force", false, "Force stop the container")
	flags.BoolVar(&force, "f", false, "Force stop (short)")
	flags.IntVar(&timeout, "timeout", 30, "Timeout in seconds before force stop")
	flags.IntVar(&timeout, "t", 30, "Timeout (short)")

	return &Command{
		Name:    "stop",
		Short:   "Stop a container",
		Usage:   "unheaded container stop <name>... [flags]",
		Aliases: []string{"down"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("at least one container name required")
			}

			client, err := getLXDClient(ctx)
			if err != nil {
				return err
			}

			for _, name := range args {
				ctx.Logger.Info().Str("name", name).Bool("force", force).Msg("stopping container")

				op, err := client.StopContainer(context.Background(), name, force, timeout)
				if err != nil {
					ctx.Output.WriteError("Failed to stop container '%s': %v", name, err)
					continue
				}

				if err := client.WaitOperation(context.Background(), op); err != nil {
					ctx.Output.WriteError("Stop operation failed for '%s': %v", name, err)
					continue
				}

				ctx.Output.WriteSuccess("Container '%s' stopped", name)
			}

			return nil
		},
	}
}

func newContainerDeleteCommand() *Command {
	var force bool

	flags := flag.NewFlagSet("delete", flag.ContinueOnError)
	flags.BoolVar(&force, "force", false, "Force delete (stop running containers)")
	flags.BoolVar(&force, "f", false, "Force delete (short)")

	return &Command{
		Name:    "delete",
		Short:   "Delete a container",
		Usage:   "unheaded container delete <name>... [flags]",
		Aliases: []string{"rm", "remove"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("at least one container name required")
			}

			client, err := getLXDClient(ctx)
			if err != nil {
				return err
			}

			for _, name := range args {
				// Check if container is running
				container, err := client.GetContainer(context.Background(), name)
				if err != nil {
					ctx.Output.WriteError("Container '%s' not found: %v", name, err)
					continue
				}

				// Stop if running and force flag is set
				if container.Status == "Running" && force {
					ctx.Logger.Info().Str("name", name).Msg("stopping container before deletion")
					op, err := client.StopContainer(context.Background(), name, true, 10)
					if err == nil {
						_ = client.WaitOperation(context.Background(), op)
					}
				} else if container.Status == "Running" && !force {
					ctx.Output.WriteError("Container '%s' is running. Use --force to stop and delete", name)
					continue
				}

				ctx.Logger.Info().Str("name", name).Msg("deleting container")

				op, err := client.DeleteContainer(context.Background(), name)
				if err != nil {
					ctx.Output.WriteError("Failed to delete container '%s': %v", name, err)
					continue
				}

				if err := client.WaitOperation(context.Background(), op); err != nil {
					ctx.Output.WriteError("Delete operation failed for '%s': %v", name, err)
					continue
				}

				ctx.Output.WriteSuccess("Container '%s' deleted", name)
			}

			return nil
		},
	}
}

func newContainerExecCommand() *Command {
	var (
		user string
		env  string
	)

	flags := flag.NewFlagSet("exec", flag.ContinueOnError)
	flags.StringVar(&user, "user", "root", "User to run command as")
	flags.StringVar(&user, "u", "root", "User (short)")
	flags.StringVar(&env, "env", "", "Environment variables (KEY=VALUE,...)")
	flags.StringVar(&env, "e", "", "Environment variables (short)")

	return &Command{
		Name:  "exec",
		Short: "Execute a command in a container",
		Usage: "unheaded container exec <name> -- <command>...",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("container name required")
			}

			name := args[0]
			var command []string

			// Find command after --
			for i, arg := range args {
				if arg == "--" {
					command = args[i+1:]
					break
				}
			}

			if len(command) == 0 {
				// Default to shell
				command = []string{"/bin/sh"}
			}

			client, err := getLXDClient(ctx)
			if err != nil {
				return err
			}

			// Parse environment variables
			envMap := make(map[string]string)
			if env != "" {
				for _, pair := range strings.Split(env, ",") {
					parts := strings.SplitN(pair, "=", 2)
					if len(parts) == 2 {
						envMap[parts[0]] = parts[1]
					}
				}
			}

			ctx.Logger.Debug().
				Str("container", name).
				Strs("command", command).
				Msg("executing command in container")

			result, err := client.ExecContainer(context.Background(), name, command, envMap)
			if err != nil {
				return fmt.Errorf("exec failed: %w", err)
			}

			// Output stdout/stderr
			if len(result.Stdout) > 0 {
				ctx.Output.WriteString(string(result.Stdout))
			}
			if len(result.Stderr) > 0 {
				ctx.Output.WriteError("%s", string(result.Stderr))
			}

			if result.ExitCode != 0 {
				return fmt.Errorf("command exited with code %d", result.ExitCode)
			}

			return nil
		},
	}
}

func newContainerInspectCommand() *Command {
	return &Command{
		Name:    "inspect",
		Short:   "Display detailed information about a container",
		Usage:   "unheaded container inspect <name>",
		Aliases: []string{"info"},
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("container name required")
			}
			name := args[0]

			client, err := getLXDClient(ctx)
			if err != nil {
				return err
			}

			container, err := client.GetContainer(context.Background(), name)
			if err != nil {
				return fmt.Errorf("container not found: %w", err)
			}

			state, err := client.GetContainerState(context.Background(), name)
			if err != nil {
				return fmt.Errorf("failed to get container state: %w", err)
			}

			// For JSON/YAML output
			if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
				data := map[string]interface{}{
					"container": container,
					"state":     state,
				}
				return ctx.Output.Write(data)
			}

			// Table output
			w := ctx.Output
			color := w.Color()

			w.WriteStringln(output.Bold("Container: " + name))
			w.WriteStringln("")

			kv := output.NewKeyValueTable(w.Out()).SetColor(color)
			kv.Add("Name", container.Name)
			kv.AddWithStatus("Status", container.Status)
			kv.Add("Architecture", container.Architecture)
			kv.Add("Created", container.CreatedAt.Format(time.RFC3339))
			kv.Add("Last Used", container.LastUsedAt.Format(time.RFC3339))
			kv.Add("Ephemeral", fmt.Sprintf("%v", container.Ephemeral))
			kv.Add("Profiles", strings.Join(container.Profiles, ", "))
			kv.Render()

			w.WriteStringln("")
			w.WriteStringln(output.Bold("Resources:"))
			w.WriteStringln("")

			resKV := output.NewKeyValueTable(w.Out()).SetColor(color).SetIndent(2)
			resKV.Add("CPU Usage", formatCPU(state.CPU.Usage))
			resKV.Add("Memory Usage", formatBytes(state.Memory.Usage))
			resKV.Add("Memory Peak", formatBytes(state.Memory.UsagePeak))
			resKV.Add("Swap Usage", formatBytes(state.Memory.SwapUsage))
			resKV.Add("PID", fmt.Sprintf("%d", state.Pid))
			resKV.Add("Processes", fmt.Sprintf("%d", state.Processes))
			resKV.Render()

			// Network interfaces
			if len(state.Network) > 0 {
				w.WriteStringln("")
				w.WriteStringln(output.Bold("Network Interfaces:"))
				for name, net := range state.Network {
					w.WriteStringln(fmt.Sprintf("\n  %s:", output.Cyan(name)))
					netKV := output.NewKeyValueTable(w.Out()).SetColor(color).SetIndent(4)
					netKV.Add("State", net.State)
					netKV.Add("Type", net.Type)
					netKV.Add("MTU", fmt.Sprintf("%d", net.Mtu))
					netKV.Add("MAC", net.Hwaddr)
					for _, addr := range net.Addresses {
						netKV.Add(addr.Family, fmt.Sprintf("%s/%s", addr.Address, addr.Netmask))
					}
					netKV.Render()
				}
			}

			return nil
		},
	}
}

// Helper functions

func getLXDClient(ctx *Context) (lxd.Client, error) {
	cfg := lxd.ClientConfig{}
	// Use mock client for CLI development/testing
	client, err := lxd.NewClient(cfg, true)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to LXD: %w", err)
	}
	if err := client.Connect(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to LXD: %w", err)
	}
	return client, nil
}

func extractIPs(c lxd.ContainerInfo) []string {
	// IPs would come from container state network info
	// For now return empty (mock client doesn't have this)
	return nil
}

func getContainerImage(c lxd.ContainerInfo) string {
	if image, ok := c.Config["image.description"]; ok {
		return image
	}
	return "unknown"
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

func formatCPU(nanoseconds int64) string {
	if nanoseconds == 0 {
		return "0s"
	}
	seconds := float64(nanoseconds) / 1e9
	if seconds < 1 {
		return fmt.Sprintf("%.2fms", seconds*1000)
	}
	return fmt.Sprintf("%.2fs", seconds)
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
