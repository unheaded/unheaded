// Package cmd provides the version command for THE GAUNTLETS CLI.
package cmd

import (
	"fmt"
	"runtime"
	"time"

	"github.com/unheaded/unheaded/cmd/unheaded-cli/output"
)

// VersionInfo contains version information for the CLI.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Compiler  string `json:"compiler"`
}

// NewVersionCommand creates the version command.
func NewVersionCommand() *Command {
	return &Command{
		Name:  "version",
		Short: "Show version information",
		Long: `Display detailed version information for the Unheaded CLI.

Shows:
  - CLI version
  - Git commit hash
  - Build timestamp
  - Go version
  - OS/Architecture`,
		Usage:   "unheaded version",
		Aliases: []string{"v"},
		RunFunc: func(ctx *Context, args []string) error {
			info := VersionInfo{
				Version:   version,
				Commit:    commit,
				BuildTime: buildTime,
				GoVersion: runtime.Version(),
				OS:        runtime.GOOS,
				Arch:      runtime.GOARCH,
				Compiler:  runtime.Compiler,
			}

			// JSON/YAML output
			if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
				return ctx.Output.Write(info)
			}

			// Table output
			w := ctx.Output
			color := w.Color()

			// Logo/Banner
			if color {
				w.WriteStringln(output.GradientText(`
  _   _       _                    _          _
 | | | |_ __ | |__   ___  __ _  __| | ___  __| |
 | | | | '_ \| '_ \ / _ \/ _' |/ _' |/ _ \/ _' |
 | |_| | | | | | | |  __/ (_| | (_| |  __/ (_| |
  \___/|_| |_|_| |_|\___|\__,_|\__,_|\___|\__,_|
`, color))
			}

			w.WriteStringln(output.Bold("THE GAUNTLETS - Unheaded Kingdom CLI"))
			w.WriteStringln("")

			kv := output.NewKeyValueTable(w.Out()).SetColor(color)
			kv.Add("Version", info.Version)
			kv.Add("Git Commit", info.Commit)
			kv.Add("Build Time", formatBuildTime(info.BuildTime))
			kv.Add("Go Version", info.GoVersion)
			kv.Add("OS/Arch", fmt.Sprintf("%s/%s", info.OS, info.Arch))
			kv.Add("Compiler", info.Compiler)
			kv.Render()

			return nil
		},
	}
}

// formatBuildTime formats the build time string.
func formatBuildTime(bt string) string {
	if bt == "unknown" {
		return bt
	}

	// Try to parse as RFC3339
	if t, err := time.Parse(time.RFC3339, bt); err == nil {
		return t.Format("2006-01-02 15:04:05 MST")
	}

	// Try to parse as Unix timestamp
	var ts int64
	if _, err := fmt.Sscanf(bt, "%d", &ts); err == nil {
		return time.Unix(ts, 0).Format("2006-01-02 15:04:05 MST")
	}

	return bt
}
