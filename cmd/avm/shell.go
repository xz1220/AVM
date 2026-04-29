package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newShellCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Manage AVM shell integration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newShellInitCommand())
	return cmd
}

func newShellInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:       "init <shell>",
		Short:     "Print AVM shell integration",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			snippet, err := shellInitSnippet(args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), snippet)
			return nil
		},
	}
}

func shellInitSnippet(shell string) (string, error) {
	switch shell {
	case "bash":
		return bashShellInitSnippet, nil
	case "zsh":
		return zshShellInitSnippet, nil
	case "fish":
		return fishShellInitSnippet, nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

const bashShellInitSnippet = `# avm shell integration
__avm_current_active() {
  local state_dir="${AVM_STATE_DIR:-$HOME/.avm/state}"
  local state_file="$state_dir/current-active"
  local active name
  [ -r "$state_file" ] || return 0
  IFS= read -r active < "$state_file" || return 0
  case "$active" in
    profile:[A-Za-z0-9_.-]*|env:[A-Za-z0-9_.-]*) name="${active#*:}" ;;
    *) return 0 ;;
  esac
  case "$name" in
    ""|*[!A-Za-z0-9_.-]*) return 0 ;;
  esac
  printf '(avm:%s) ' "$name"
}

if [ -z "${__AVM_ORIGINAL_PS1+x}" ]; then
  __AVM_ORIGINAL_PS1="${PS1-}"
fi

__avm_prompt_command() {
  local avm_active
  avm_active="$(__avm_current_active)"
  PS1="${avm_active}${__AVM_ORIGINAL_PS1}"
}

case ";${PROMPT_COMMAND:-};" in
  *";__avm_prompt_command;"*) ;;
  *) PROMPT_COMMAND="__avm_prompt_command${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
esac

avm() {
  if [ "$#" -gt 0 ] && [ "$1" = "use" ]; then
    shift
    local __avm_output __avm_status
    if __avm_output="$(command avm activate "$@")"; then
      eval "$__avm_output"
      printf 'active: %s\n' "$AVM_ACTIVE"
      return 0
    fi
    __avm_status=$?
    [ -z "$__avm_output" ] || printf '%s\n' "$__avm_output" >&2
    return "$__avm_status"
  fi
  command avm "$@"
}

claude() {
  if [ -n "${AVM_CLAUDE_MCP_CONFIG:-}" ] && [ -r "$AVM_CLAUDE_MCP_CONFIG" ]; then
    case " $* " in
      *" --mcp-config "*|*" --mcp-config="*) command claude "$@" ;;
      *) command claude --strict-mcp-config --mcp-config="$AVM_CLAUDE_MCP_CONFIG" "$@" ;;
    esac
  else
    command claude "$@"
  fi
}
`

const zshShellInitSnippet = `# avm shell integration
__avm_current_active() {
  local state_dir="${AVM_STATE_DIR:-$HOME/.avm/state}"
  local state_file="$state_dir/current-active"
  local active name
  [ -r "$state_file" ] || return 0
  IFS= read -r active < "$state_file" || return 0
  case "$active" in
    profile:[A-Za-z0-9_.-]*|env:[A-Za-z0-9_.-]*) name="${active#*:}" ;;
    *) return 0 ;;
  esac
  case "$name" in
    ""|*[!A-Za-z0-9_.-]*) return 0 ;;
  esac
  printf '(avm:%s) ' "$name"
}

if [ -z "${__AVM_ORIGINAL_PROMPT+x}" ]; then
  __AVM_ORIGINAL_PROMPT="${PROMPT-}"
fi

__avm_precmd() {
  local avm_active
  avm_active="$(__avm_current_active)"
  PROMPT="${avm_active}${__AVM_ORIGINAL_PROMPT}"
}

autoload -Uz add-zsh-hook
add-zsh-hook -d precmd __avm_precmd 2>/dev/null || true
add-zsh-hook precmd __avm_precmd

avm() {
  if [ "$#" -gt 0 ] && [ "$1" = "use" ]; then
    shift
    local __avm_output __avm_status
    if __avm_output="$(command avm activate "$@")"; then
      eval "$__avm_output"
      printf 'active: %s\n' "$AVM_ACTIVE"
      return 0
    fi
    __avm_status=$?
    [ -z "$__avm_output" ] || printf '%s\n' "$__avm_output" >&2
    return "$__avm_status"
  fi
  command avm "$@"
}

claude() {
  if [ -n "${AVM_CLAUDE_MCP_CONFIG:-}" ] && [ -r "$AVM_CLAUDE_MCP_CONFIG" ]; then
    case " $* " in
      *" --mcp-config "*|*" --mcp-config="*) command claude "$@" ;;
      *) command claude --strict-mcp-config --mcp-config="$AVM_CLAUDE_MCP_CONFIG" "$@" ;;
    esac
  else
    command claude "$@"
  fi
}
`

const fishShellInitSnippet = `# avm shell integration
function __avm_current_active
    set -l state_dir "$HOME/.avm/state"
    if set -q AVM_STATE_DIR
        set state_dir "$AVM_STATE_DIR"
    end
    set -l state_file "$state_dir/current-active"
    test -r "$state_file"; or return 0
    read -l active < "$state_file"; or return 0
    switch "$active"
        case 'profile:*' 'env:*'
            set -l name (string replace -r '^[^:]*:' '' -- "$active")
            if string match -qr '^[A-Za-z0-9_.-]+$' -- "$name"
                printf '(avm:%s) ' "$name"
            end
    end
end

if functions -q fish_prompt; and not functions -q __avm_original_fish_prompt
    functions -c fish_prompt __avm_original_fish_prompt
end

if not functions -q __avm_original_fish_prompt
    function __avm_original_fish_prompt
        printf '> '
    end
end

function fish_prompt
    __avm_current_active
    __avm_original_fish_prompt
end

function avm
    if test (count $argv) -gt 0; and test "$argv[1]" = use
        set -e argv[1]
        set -l __avm_output (command avm activate $argv | string collect)
        set -l __avm_status $pipestatus[1]
        if test "$__avm_status" -eq 0
            eval "$__avm_output"
            printf 'active: %s\n' "$AVM_ACTIVE"
            return 0
        end
        test -z "$__avm_output"; or printf '%s\n' "$__avm_output" >&2
        return "$__avm_status"
    end
    command avm $argv
end

function claude
    if set -q AVM_CLAUDE_MCP_CONFIG; and test -r "$AVM_CLAUDE_MCP_CONFIG"
        set -l has_mcp_config 0
        for arg in $argv
            switch $arg
                case --mcp-config '--mcp-config=*'
                    set has_mcp_config 1
            end
        end
        if test "$has_mcp_config" = 1
            command claude $argv
        else
            command claude --strict-mcp-config --mcp-config="$AVM_CLAUDE_MCP_CONFIG" $argv
        end
    else
        command claude $argv
    end
end
`
