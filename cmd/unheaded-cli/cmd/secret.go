// Package cmd provides secret commands for THE GAUNTLETS CLI.
package cmd

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"unheaded/cmd/unheaded-cli/output"
	"unheaded/pkg/secrets"
	"unheaded/pkg/secrets/store"
)

// SecretListResult represents the output for secret list.
type SecretListResult struct {
	Secrets []SecretInfo `json:"secrets"`
}

// SecretInfo represents secret information for output.
type SecretInfo struct {
	Path      string    `json:"path"`
	Type      string    `json:"type"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// WriteTable implements output.TableWriter.
func (r *SecretListResult) WriteTable(t *output.Table, wide bool) {
	if wide {
		t.SetHeaders("PATH", "TYPE", "VERSION", "CREATED", "UPDATED", "EXPIRES")
	} else {
		t.SetHeaders("PATH", "TYPE", "VERSION", "UPDATED")
	}

	for _, s := range r.Secrets {
		updated := formatAge(s.UpdatedAt)

		if wide {
			created := formatAge(s.CreatedAt)
			expires := "-"
			if !s.ExpiresAt.IsZero() {
				expires = s.ExpiresAt.Format(time.RFC3339)
			}
			t.AddRow(s.Path, s.Type, fmt.Sprintf("%d", s.Version), created, updated, expires)
		} else {
			t.AddRow(s.Path, s.Type, fmt.Sprintf("%d", s.Version), updated)
		}
	}
}

// NewSecretCommand creates the secret command.
func NewSecretCommand() *Command {
	cmd := &Command{
		Name:        "secret",
		Short:       "Manage secrets",
		Long:        "Commands for managing secrets in the Unheaded Kingdom's Crystal Grotto.",
		Aliases:     []string{"secrets"},
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newSecretGetCommand())
	cmd.AddCommand(newSecretSetCommand())
	cmd.AddCommand(newSecretDeleteCommand())
	cmd.AddCommand(newSecretListCommand())
	cmd.AddCommand(newSecretRotateCommand())
	cmd.AddCommand(newSecretGenerateCommand())

	return cmd
}

func newSecretGetCommand() *Command {
	var (
		version int64
		key     string
	)

	flags := flag.NewFlagSet("get", flag.ContinueOnError)
	flags.Int64Var(&version, "version", 0, "Specific version to retrieve (0 = latest)")
	flags.Int64Var(&version, "v", 0, "Version (short)")
	flags.StringVar(&key, "key", "", "Specific key to retrieve from the secret data")
	flags.StringVar(&key, "k", "", "Key (short)")

	return &Command{
		Name:  "get",
		Short: "Get a secret",
		Usage: "unheaded secret get <path> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("secret path required")
			}
			path := args[0]

			manager, err := getSecretsManager(ctx)
			if err != nil {
				return err
			}
			defer manager.Close()

			secret, err := manager.Get(context.Background(), path)
			if err != nil {
				return fmt.Errorf("failed to get secret: %w", err)
			}

			// If specific key requested, only return that
			if key != "" {
				data, ok := secret.Data[key]
				if !ok {
					return fmt.Errorf("key '%s' not found in secret", key)
				}
				ctx.Output.WriteStringln(string(data))
				return nil
			}

			// For JSON/YAML output, include all data (masked)
			if ctx.Output.Format() == output.FormatJSON || ctx.Output.Format() == output.FormatYAML {
				// Mask sensitive values in output
				maskedData := make(map[string]string)
				for k := range secret.Data {
					maskedData[k] = "***REDACTED***"
				}
				result := map[string]interface{}{
					"path":       secret.Path,
					"type":       secret.Type,
					"version":    secret.Version,
					"created_at": secret.CreatedAt,
					"updated_at": secret.UpdatedAt,
					"data_keys":  maskedData,
				}
				return ctx.Output.Write(result)
			}

			// Table output
			w := ctx.Output
			color := w.Color()

			w.WriteStringln(output.Bold("Secret: " + path))
			w.WriteStringln("")

			kv := output.NewKeyValueTable(w.Out()).SetColor(color)
			kv.Add("Path", secret.Path)
			kv.Add("Type", string(secret.Type))
			kv.Add("Version", fmt.Sprintf("%d", secret.Version))
			kv.Add("Created", secret.CreatedAt.Format(time.RFC3339))
			kv.Add("Updated", secret.UpdatedAt.Format(time.RFC3339))
			if !secret.ExpiresAt.IsZero() {
				kv.Add("Expires", secret.ExpiresAt.Format(time.RFC3339))
			}
			kv.Render()

			w.WriteStringln("")
			w.WriteStringln(output.Bold("Data Keys:"))
			list := output.NewListTable(w.Out()).SetColor(color)
			for k := range secret.Data {
				list.Add(k)
			}
			list.Render()

			w.WriteStringln("")
			w.WriteInfo("Use 'unheaded secret get %s --key <key>' to retrieve specific values", path)

			return nil
		},
	}
}

func newSecretSetCommand() *Command {
	var (
		secretType string
		expiresIn  string
		fromFile   string
	)

	flags := flag.NewFlagSet("set", flag.ContinueOnError)
	flags.StringVar(&secretType, "type", "static", "Secret type: static, dynamic, certificate, ssh_key, kek")
	flags.StringVar(&secretType, "t", "static", "Type (short)")
	flags.StringVar(&expiresIn, "expires", "", "Expiration duration (e.g., 24h, 7d)")
	flags.StringVar(&expiresIn, "e", "", "Expires (short)")
	flags.StringVar(&fromFile, "from-file", "", "Load secret value from file")
	flags.StringVar(&fromFile, "f", "", "From file (short)")

	return &Command{
		Name:  "set",
		Short: "Set a secret",
		Usage: "unheaded secret set <path> <key>=<value>... [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("secret path and at least one key=value pair required")
			}
			path := args[0]
			keyValues := args[1:]

			// Parse key-value pairs
			data := make(map[string][]byte)
			for _, kv := range keyValues {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid key=value pair: %s", kv)
				}
				data[parts[0]] = []byte(parts[1])
			}

			// Parse secret type
			var st secrets.SecretType
			switch secretType {
			case "static":
				st = secrets.SecretTypeStatic
			case "dynamic":
				st = secrets.SecretTypeDynamic
			case "certificate":
				st = secrets.SecretTypeCertificate
			case "ssh_key":
				st = secrets.SecretTypeSSHKey
			case "kek":
				st = secrets.SecretTypeKEK
			default:
				return fmt.Errorf("invalid secret type: %s", secretType)
			}

			manager, err := getSecretsManager(ctx)
			if err != nil {
				return err
			}
			defer manager.Close()

			secret := &secrets.Secret{
				Path: path,
				Type: st,
				Data: data,
			}

			// Parse expiration
			if expiresIn != "" {
				duration, err := parseDuration(expiresIn)
				if err != nil {
					return fmt.Errorf("invalid expiration: %w", err)
				}
				secret.ExpiresAt = time.Now().Add(duration)
			}

			ctx.Logger.Info().Str("path", path).Str("type", secretType).Msg("setting secret")

			if err := manager.Set(context.Background(), path, secret); err != nil {
				return fmt.Errorf("failed to set secret: %w", err)
			}

			ctx.Output.WriteSuccess("Secret '%s' saved (version %d)", path, secret.Version)
			return nil
		},
	}
}

func newSecretDeleteCommand() *Command {
	var force bool

	flags := flag.NewFlagSet("delete", flag.ContinueOnError)
	flags.BoolVar(&force, "force", false, "Force deletion without confirmation")
	flags.BoolVar(&force, "f", false, "Force (short)")

	return &Command{
		Name:    "delete",
		Short:   "Delete a secret",
		Usage:   "unheaded secret delete <path> [flags]",
		Aliases: []string{"rm"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("secret path required")
			}
			path := args[0]

			if !force {
				ctx.Output.WriteWarning("This will permanently delete secret '%s'. Use --force to confirm.", path)
				return nil
			}

			manager, err := getSecretsManager(ctx)
			if err != nil {
				return err
			}
			defer manager.Close()

			ctx.Logger.Info().Str("path", path).Msg("deleting secret")

			if err := manager.Delete(context.Background(), path); err != nil {
				return fmt.Errorf("failed to delete secret: %w", err)
			}

			ctx.Output.WriteSuccess("Secret '%s' deleted", path)
			return nil
		},
	}
}

func newSecretListCommand() *Command {
	var prefix string

	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.StringVar(&prefix, "prefix", "", "Filter by path prefix")
	flags.StringVar(&prefix, "p", "", "Prefix (short)")

	return &Command{
		Name:    "list",
		Short:   "List secrets",
		Usage:   "unheaded secret list [flags]",
		Aliases: []string{"ls"},
		Flags:   flags,
		RunFunc: func(ctx *Context, args []string) error {
			manager, err := getSecretsManager(ctx)
			if err != nil {
				return err
			}
			defer manager.Close()

			paths, err := manager.List(context.Background(), prefix)
			if err != nil {
				return fmt.Errorf("failed to list secrets: %w", err)
			}

			result := &SecretListResult{}
			for _, path := range paths {
				secret, err := manager.Get(context.Background(), path)
				if err != nil {
					continue // Skip errors for individual secrets
				}
				result.Secrets = append(result.Secrets, SecretInfo{
					Path:      secret.Path,
					Type:      string(secret.Type),
					Version:   secret.Version,
					CreatedAt: secret.CreatedAt,
					UpdatedAt: secret.UpdatedAt,
					ExpiresAt: secret.ExpiresAt,
				})
			}

			if len(result.Secrets) == 0 {
				ctx.Output.WriteInfo("No secrets found")
				return nil
			}

			return ctx.Output.Write(result)
		},
	}
}

func newSecretRotateCommand() *Command {
	var (
		keepVersions int
		force        bool
	)

	flags := flag.NewFlagSet("rotate", flag.ContinueOnError)
	flags.IntVar(&keepVersions, "keep", 3, "Number of versions to keep")
	flags.IntVar(&keepVersions, "k", 3, "Keep versions (short)")
	flags.BoolVar(&force, "force", false, "Force rotation even if not expired")
	flags.BoolVar(&force, "f", false, "Force (short)")

	return &Command{
		Name:  "rotate",
		Short: "Rotate a secret",
		Usage: "unheaded secret rotate <path> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("secret path required")
			}
			path := args[0]

			manager, err := getSecretsManager(ctx)
			if err != nil {
				return err
			}
			defer manager.Close()

			// Get existing secret
			secret, err := manager.Get(context.Background(), path)
			if err != nil {
				return fmt.Errorf("failed to get secret for rotation: %w", err)
			}

			// Check if rotation is needed
			if !force && !secret.IsExpired() {
				ctx.Output.WriteInfo("Secret '%s' has not expired. Use --force to rotate anyway.", path)
				return nil
			}

			ctx.Logger.Info().Str("path", path).Msg("rotating secret")

			// For static secrets, we need the user to provide a new value
			// For dynamic secrets, regenerate
			if secret.Type == secrets.SecretTypeStatic {
				return fmt.Errorf("static secrets require manual rotation. Use 'unheaded secret set' with new values")
			}

			// In a real implementation, this would trigger the rotation mechanism
			ctx.Output.WriteWarning("Dynamic secret rotation not yet implemented")
			return nil
		},
	}
}

func newSecretGenerateCommand() *Command {
	var (
		length     int
		charset    string
		secretType string
	)

	flags := flag.NewFlagSet("generate", flag.ContinueOnError)
	flags.IntVar(&length, "length", 32, "Length of generated secret")
	flags.IntVar(&length, "l", 32, "Length (short)")
	flags.StringVar(&charset, "charset", "alphanumeric", "Character set: alphanumeric, alphanumeric-special, hex")
	flags.StringVar(&charset, "c", "alphanumeric", "Charset (short)")
	flags.StringVar(&secretType, "type", "static", "Secret type: static, kek")
	flags.StringVar(&secretType, "t", "static", "Type (short)")

	return &Command{
		Name:  "generate",
		Short: "Generate a random secret",
		Usage: "unheaded secret generate <path> [flags]",
		Flags: flags,
		RunFunc: func(ctx *Context, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("secret path required")
			}
			path := args[0]

			// Generate random value
			value, err := secrets.GenerateRandomString(length)
			if err != nil {
				return fmt.Errorf("failed to generate secret: %w", err)
			}

			// Parse secret type
			var st secrets.SecretType
			switch secretType {
			case "static":
				st = secrets.SecretTypeStatic
			case "kek":
				st = secrets.SecretTypeKEK
			default:
				return fmt.Errorf("generate only supports 'static' and 'kek' types")
			}

			manager, err := getSecretsManager(ctx)
			if err != nil {
				return err
			}
			defer manager.Close()

			secret := &secrets.Secret{
				Path: path,
				Type: st,
				Data: map[string][]byte{
					"value": []byte(value),
				},
			}

			ctx.Logger.Info().Str("path", path).Int("length", length).Msg("generating secret")

			if err := manager.Set(context.Background(), path, secret); err != nil {
				return fmt.Errorf("failed to save generated secret: %w", err)
			}

			ctx.Output.WriteSuccess("Generated and saved secret '%s' (length=%d)", path, length)
			return nil
		},
	}
}

// getSecretsManager creates a secrets manager instance.
func getSecretsManager(ctx *Context) (*secrets.Manager, error) {
	// Create memory store for demo (real impl would use file or Vault)
	memStore := store.NewMemoryStore(store.MemoryStoreConfig{
		Logger: ctx.Logger,
	})

	manager, err := secrets.NewManager(secrets.ManagerConfig{
		Store:  memStore,
		Logger: ctx.Logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create secrets manager: %w", err)
	}

	return manager, nil
}

// parseDuration parses a duration string supporting days (e.g., "7d").
func parseDuration(s string) (time.Duration, error) {
	// Handle day suffix
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, fmt.Errorf("invalid days: %s", days)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
