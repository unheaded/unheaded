// Package cmd provides observability commands for THE GAUNTLETS CLI.
package cmd

import (
	"flag"
	"fmt"
	"os"

	"unheaded/pkg/observability"
)

// NewObserveCommand creates the observe command.
func NewObserveCommand() *Command {
	cmd := &Command{
		Name:    "observe",
		Short:   "Manage observability backends",
		Long:    "Configure and inspect observability backend adapters.\n\nSupported backends: prometheus, grafana, elk, fluentd, jaeger, nagios, loki, wotan",
		Aliases: []string{"obs"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newObserveListCommand())
	cmd.AddCommand(newObserveConfigCommand())
	cmd.AddCommand(newObserveDashboardCommand())

	return cmd
}

func newObserveListCommand() *Command {
	return &Command{
		Name:    "list",
		Short:   "List available observability backends",
		Aliases: []string{"ls"},
		RunFunc: func(ctx *Context, args []string) error {
			fmt.Fprintln(os.Stdout, "Available observability backends:")
			fmt.Fprintln(os.Stdout, "")
			fmt.Fprintf(os.Stdout, "  %-14s  %-8s  %s\n", "BACKEND", "STATUS", "SIGNALS")
			fmt.Fprintf(os.Stdout, "  %-14s  %-8s  %s\n", "-------", "------", "-------")

			for _, b := range observability.AllBackends() {
				status, signals := backendInfo(b)
				fmt.Fprintf(os.Stdout, "  %-14s  %-8s  %s\n", b, status, signals)
			}
			return nil
		},
	}
}

func newObserveConfigCommand() *Command {
	flags := flag.NewFlagSet("config", flag.ContinueOnError)
	backend := flags.String("backend", "", "Backend to generate config for")
	service := flags.String("service", "", "Service name")
	host := flags.String("host", "", "Backend host (e.g. localhost:9200)")

	return &Command{
		Name:  "config",
		Short: "Generate backend configuration files",
		Usage: "unheaded observe config --backend <backend> --service <service> [--host <host>]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if *backend == "" || *service == "" {
				return fmt.Errorf("--backend and --service are required")
			}

			var config string
			switch observability.Backend(*backend) {
			case observability.BackendELK:
				config = observability.LogstashConfig(*service, *host)
			case observability.BackendFluentd:
				config = observability.FluentdConfig(*service, *host)
			case observability.BackendLoki:
				config = observability.PromtailConfig(*service, *host)
			case observability.BackendJaeger:
				config = observability.JaegerCollectorConfig(*host)
			case observability.BackendNagios:
				port := servicePort(*service)
				config = observability.NagiosServiceDefinition(*service, port)
			default:
				return fmt.Errorf("config generation not supported for backend %q", *backend)
			}

			fmt.Fprint(os.Stdout, config)
			return nil
		},
	}
}

func newObserveDashboardCommand() *Command {
	flags := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	service := flags.String("service", "", "Service to generate dashboard for")

	return &Command{
		Name:  "dashboard",
		Short: "Generate Grafana dashboard JSON",
		Usage: "unheaded observe dashboard --service <service>",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if *service == "" {
				return fmt.Errorf("--service is required")
			}

			adapter := observability.NewGrafanaAdapter()
			adapter.GenerateServiceDashboard(*service)

			json, err := adapter.DashboardJSON(fmt.Sprintf("unheaded-%s", *service))
			if err != nil {
				return err
			}

			fmt.Fprintln(os.Stdout, json)
			return nil
		},
	}
}

func backendInfo(b observability.Backend) (status, signals string) {
	switch b {
	case observability.BackendPrometheus:
		return "active", "metrics"
	case observability.BackendGrafana:
		return "active", "metrics, alerts"
	case observability.BackendELK:
		return "ready", "logs"
	case observability.BackendFluentd:
		return "ready", "logs"
	case observability.BackendJaeger:
		return "ready", "traces"
	case observability.BackendNagios:
		return "ready", "alerts, metrics"
	case observability.BackendLoki:
		return "ready", "logs"
	case observability.BackendWotan:
		return "planned", "metrics, logs, traces, alerts"
	default:
		return "unknown", ""
	}
}

func servicePort(name string) int {
	ports := map[string]int{
		"wotan":            18000,
		"timeguru":         19000,
		"architect":        19001,
		"captain":          19002,
		"micromanager":     19003,
		"monad":            19004,
		"sophia":           19005,
		"dashboard-backend": 20000,
		"kanban-app":       20001,
		"unheaded-daemon":  17000,
	}
	if p, ok := ports[name]; ok {
		return p
	}
	return 8080
}
