// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cmd provides shell completion for THE GAUNTLETS CLI.
package cmd


// NewCompletionCommand creates the completion command.
func NewCompletionCommand() *Command {
	cmd := &Command{
		Name:  "completion",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for bash, zsh, or fish.

To load completions:

Bash:
  $ source <(unheaded completion bash)
  # To load completions for each session, execute once:
  $ unheaded completion bash > /etc/bash_completion.d/unheaded

Zsh:
  $ source <(unheaded completion zsh)
  # To load completions for each session, execute once:
  $ unheaded completion zsh > "${fpath[1]}/_unheaded"

Fish:
  $ unheaded completion fish | source
  # To load completions for each session, execute once:
  $ unheaded completion fish > ~/.config/fish/completions/unheaded.fish`,
		Usage:       "unheaded completion <shell>",
		SubCommands: make(map[string]*Command),
	}

	cmd.AddCommand(newBashCompletionCommand())
	cmd.AddCommand(newZshCompletionCommand())
	cmd.AddCommand(newFishCompletionCommand())

	return cmd
}

func newBashCompletionCommand() *Command {
	return &Command{
		Name:  "bash",
		Short: "Generate bash completion script",
		Usage: "unheaded completion bash",
		RunFunc: func(ctx *Context, args []string) error {
			script := generateBashCompletion()
			ctx.Output.WriteString(script)
			return nil
		},
	}
}

func newZshCompletionCommand() *Command {
	return &Command{
		Name:  "zsh",
		Short: "Generate zsh completion script",
		Usage: "unheaded completion zsh",
		RunFunc: func(ctx *Context, args []string) error {
			script := generateZshCompletion()
			ctx.Output.WriteString(script)
			return nil
		},
	}
}

func newFishCompletionCommand() *Command {
	return &Command{
		Name:  "fish",
		Short: "Generate fish completion script",
		Usage: "unheaded completion fish",
		RunFunc: func(ctx *Context, args []string) error {
			script := generateFishCompletion()
			ctx.Output.WriteString(script)
			return nil
		},
	}
}

func generateBashCompletion() string {
	return `# bash completion for unheaded

_unheaded_completions() {
    local cur prev words cword
    _init_completion || return

    local commands="container service config secret network status version completion"
    local container_commands="list create start stop delete exec inspect"
    local service_commands="list deploy status logs restart scale"
    local config_commands="get set list validate current-context use-context view init"
    local secret_commands="get set delete list rotate generate"
    local network_commands="policy route inspect"
    local policy_commands="list create delete apply"
    local route_commands="list add delete"

    case "${prev}" in
        unheaded)
            COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
            return 0
            ;;
        container|ct|containers)
            COMPREPLY=($(compgen -W "${container_commands}" -- "${cur}"))
            return 0
            ;;
        service|svc|services)
            COMPREPLY=($(compgen -W "${service_commands}" -- "${cur}"))
            return 0
            ;;
        config|cfg)
            COMPREPLY=($(compgen -W "${config_commands}" -- "${cur}"))
            return 0
            ;;
        secret|secrets)
            COMPREPLY=($(compgen -W "${secret_commands}" -- "${cur}"))
            return 0
            ;;
        network|net)
            COMPREPLY=($(compgen -W "${network_commands}" -- "${cur}"))
            return 0
            ;;
        policy|pol|policies)
            COMPREPLY=($(compgen -W "${policy_commands}" -- "${cur}"))
            return 0
            ;;
        route|routes)
            COMPREPLY=($(compgen -W "${route_commands}" -- "${cur}"))
            return 0
            ;;
        -o|--output)
            COMPREPLY=($(compgen -W "table json yaml wide" -- "${cur}"))
            return 0
            ;;
        --context)
            # Contexts loaded from config
            COMPREPLY=($(compgen -W "local staging production" -- "${cur}"))
            return 0
            ;;
    esac

    # Global flags
    if [[ "${cur}" == -* ]]; then
        COMPREPLY=($(compgen -W "-o --output -v --verbose -q --quiet --no-color --context --help" -- "${cur}"))
        return 0
    fi
}

complete -F _unheaded_completions unheaded
`
}

func generateZshCompletion() string {
	return `#compdef unheaded

_unheaded() {
    local -a commands
    local -a container_commands
    local -a service_commands
    local -a config_commands
    local -a secret_commands
    local -a network_commands

    commands=(
        'container:Manage containers'
        'service:Manage services'
        'config:Manage CLI configuration'
        'secret:Manage secrets'
        'network:Manage network policies and routes'
        'status:Show system status overview'
        'version:Show version information'
        'completion:Generate shell completion scripts'
    )

    container_commands=(
        'list:List containers'
        'create:Create a new container'
        'start:Start a container'
        'stop:Stop a container'
        'delete:Delete a container'
        'exec:Execute a command in a container'
        'inspect:Display detailed information about a container'
    )

    service_commands=(
        'list:List services'
        'deploy:Deploy a service'
        'status:Show service status'
        'logs:View service logs'
        'restart:Restart a service'
        'scale:Scale a service'
    )

    config_commands=(
        'get:Get a configuration value'
        'set:Set a configuration value'
        'list:List all configuration values'
        'validate:Validate configuration'
        'current-context:Display the current context'
        'use-context:Set the current context'
        'view:View the full configuration'
        'init:Initialize CLI configuration'
    )

    secret_commands=(
        'get:Get a secret'
        'set:Set a secret'
        'delete:Delete a secret'
        'list:List secrets'
        'rotate:Rotate a secret'
        'generate:Generate a random secret'
    )

    network_commands=(
        'policy:Manage network policies'
        'route:Manage network routes'
        'inspect:Inspect network configuration'
    )

    _arguments -C \
        '-o[Output format]:format:(table json yaml wide)' \
        '--output[Output format]:format:(table json yaml wide)' \
        '-v[Enable verbose output]' \
        '--verbose[Enable verbose output]' \
        '-q[Suppress non-essential output]' \
        '--quiet[Suppress non-essential output]' \
        '--no-color[Disable color output]' \
        '--context[Context to use]:context:->contexts' \
        '1: :->command' \
        '*:: :->args'

    case $state in
        command)
            _describe -t commands 'unheaded command' commands
            ;;
        args)
            case $words[1] in
                container|ct|containers)
                    if (( CURRENT == 2 )); then
                        _describe -t commands 'container command' container_commands
                    fi
                    ;;
                service|svc|services)
                    if (( CURRENT == 2 )); then
                        _describe -t commands 'service command' service_commands
                    fi
                    ;;
                config|cfg)
                    if (( CURRENT == 2 )); then
                        _describe -t commands 'config command' config_commands
                    fi
                    ;;
                secret|secrets)
                    if (( CURRENT == 2 )); then
                        _describe -t commands 'secret command' secret_commands
                    fi
                    ;;
                network|net)
                    if (( CURRENT == 2 )); then
                        _describe -t commands 'network command' network_commands
                    fi
                    ;;
            esac
            ;;
        contexts)
            # Contexts loaded from config
            local -a contexts
            contexts=(local staging production)
            _describe -t contexts 'context' contexts
            ;;
    esac
}

_unheaded "$@"
`
}

func generateFishCompletion() string {
	return `# fish completion for unheaded

# Main commands
complete -c unheaded -f -n "__fish_use_subcommand" -a "container" -d "Manage containers"
complete -c unheaded -f -n "__fish_use_subcommand" -a "service" -d "Manage services"
complete -c unheaded -f -n "__fish_use_subcommand" -a "config" -d "Manage CLI configuration"
complete -c unheaded -f -n "__fish_use_subcommand" -a "secret" -d "Manage secrets"
complete -c unheaded -f -n "__fish_use_subcommand" -a "network" -d "Manage network policies and routes"
complete -c unheaded -f -n "__fish_use_subcommand" -a "status" -d "Show system status overview"
complete -c unheaded -f -n "__fish_use_subcommand" -a "version" -d "Show version information"
complete -c unheaded -f -n "__fish_use_subcommand" -a "completion" -d "Generate shell completion scripts"

# Container subcommands
complete -c unheaded -f -n "__fish_seen_subcommand_from container ct containers" -a "list" -d "List containers"
complete -c unheaded -f -n "__fish_seen_subcommand_from container ct containers" -a "create" -d "Create a new container"
complete -c unheaded -f -n "__fish_seen_subcommand_from container ct containers" -a "start" -d "Start a container"
complete -c unheaded -f -n "__fish_seen_subcommand_from container ct containers" -a "stop" -d "Stop a container"
complete -c unheaded -f -n "__fish_seen_subcommand_from container ct containers" -a "delete" -d "Delete a container"
complete -c unheaded -f -n "__fish_seen_subcommand_from container ct containers" -a "exec" -d "Execute a command in a container"
complete -c unheaded -f -n "__fish_seen_subcommand_from container ct containers" -a "inspect" -d "Display detailed information"

# Service subcommands
complete -c unheaded -f -n "__fish_seen_subcommand_from service svc services" -a "list" -d "List services"
complete -c unheaded -f -n "__fish_seen_subcommand_from service svc services" -a "deploy" -d "Deploy a service"
complete -c unheaded -f -n "__fish_seen_subcommand_from service svc services" -a "status" -d "Show service status"
complete -c unheaded -f -n "__fish_seen_subcommand_from service svc services" -a "logs" -d "View service logs"
complete -c unheaded -f -n "__fish_seen_subcommand_from service svc services" -a "restart" -d "Restart a service"
complete -c unheaded -f -n "__fish_seen_subcommand_from service svc services" -a "scale" -d "Scale a service"

# Config subcommands
complete -c unheaded -f -n "__fish_seen_subcommand_from config cfg" -a "get" -d "Get a configuration value"
complete -c unheaded -f -n "__fish_seen_subcommand_from config cfg" -a "set" -d "Set a configuration value"
complete -c unheaded -f -n "__fish_seen_subcommand_from config cfg" -a "list" -d "List all configuration values"
complete -c unheaded -f -n "__fish_seen_subcommand_from config cfg" -a "validate" -d "Validate configuration"
complete -c unheaded -f -n "__fish_seen_subcommand_from config cfg" -a "current-context" -d "Display the current context"
complete -c unheaded -f -n "__fish_seen_subcommand_from config cfg" -a "use-context" -d "Set the current context"
complete -c unheaded -f -n "__fish_seen_subcommand_from config cfg" -a "view" -d "View the full configuration"
complete -c unheaded -f -n "__fish_seen_subcommand_from config cfg" -a "init" -d "Initialize CLI configuration"

# Secret subcommands
complete -c unheaded -f -n "__fish_seen_subcommand_from secret secrets" -a "get" -d "Get a secret"
complete -c unheaded -f -n "__fish_seen_subcommand_from secret secrets" -a "set" -d "Set a secret"
complete -c unheaded -f -n "__fish_seen_subcommand_from secret secrets" -a "delete" -d "Delete a secret"
complete -c unheaded -f -n "__fish_seen_subcommand_from secret secrets" -a "list" -d "List secrets"
complete -c unheaded -f -n "__fish_seen_subcommand_from secret secrets" -a "rotate" -d "Rotate a secret"
complete -c unheaded -f -n "__fish_seen_subcommand_from secret secrets" -a "generate" -d "Generate a random secret"

# Network subcommands
complete -c unheaded -f -n "__fish_seen_subcommand_from network net" -a "policy" -d "Manage network policies"
complete -c unheaded -f -n "__fish_seen_subcommand_from network net" -a "route" -d "Manage network routes"
complete -c unheaded -f -n "__fish_seen_subcommand_from network net" -a "inspect" -d "Inspect network configuration"

# Global flags
complete -c unheaded -s o -l output -d "Output format" -a "table json yaml wide"
complete -c unheaded -s v -l verbose -d "Enable verbose output"
complete -c unheaded -s q -l quiet -d "Suppress non-essential output"
complete -c unheaded -l no-color -d "Disable color output"
complete -c unheaded -l context -d "Context to use"
complete -c unheaded -s h -l help -d "Show help"

# Completion subcommands
complete -c unheaded -f -n "__fish_seen_subcommand_from completion" -a "bash" -d "Generate bash completion"
complete -c unheaded -f -n "__fish_seen_subcommand_from completion" -a "zsh" -d "Generate zsh completion"
complete -c unheaded -f -n "__fish_seen_subcommand_from completion" -a "fish" -d "Generate fish completion"
`
}
