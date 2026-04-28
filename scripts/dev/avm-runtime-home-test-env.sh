#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
STATE_DIR="$(git -C "$REPO_ROOT" rev-parse --git-path avm-runtime-home-test-env 2>/dev/null || printf '%s/.avm-runtime-home-test-env' "$REPO_ROOT")"
STATE_FILE="$STATE_DIR/state"

CODEX_MODEL="${AVM_TEST_CODEX_MODEL:-gpt-5.4-mini}"
CLAUDE_MODEL="${AVM_TEST_CLAUDE_MODEL:-sonnet}"
OPENCODE_MODEL="${AVM_TEST_OPENCODE_MODEL:-anthropic/claude-sonnet-4-5}"
SUPERPOWERS_REPO="${AVM_TEST_SUPERPOWERS_REPO:-https://github.com/obra/superpowers.git}"
SUPERPOWERS_REF="${AVM_TEST_SUPERPOWERS_REF:-main}"

usage() {
  cat <<'EOF'
Usage:
  scripts/dev/avm-runtime-home-test-env.sh start   # create if needed, then enter test shell
  scripts/dev/avm-runtime-home-test-env.sh create  # create test env only
  scripts/dev/avm-runtime-home-test-env.sh enter   # enter existing test shell
  scripts/dev/avm-runtime-home-test-env.sh status  # show current test env
  scripts/dev/avm-runtime-home-test-env.sh delete  # delete test env

Options:
  AVM_TEST_SHELL=zsh|bash       Override the shell used by start/enter.
  AVM_TEST_CLAUDE_MODEL=sonnet  Override the Claude Code agent model.
  AVM_TEST_OPENCODE_MODEL=...    Override the OpenCode agent model.

Inside the test shell, use AVM normally:
  avm agent list
  avm agent show codex-agent
  avm env create review --codex codex-agent
  eval "$(avm activate --kind env coding)"
  codex exec --skip-git-repo-check --ephemeral "Reply exactly AVM_CODEX_OK."
  # In interactive codex, run /mcp to inspect the filesystem MCP server.
  claude agents
  opencode
EOF
}

shell_quote() {
  printf "%q" "$1"
}

load_state() {
  if [ ! -f "$STATE_FILE" ]; then
    return 1
  fi
  # shellcheck disable=SC1090
  source "$STATE_FILE"
}

save_state() {
  mkdir -p "$STATE_DIR"
  {
    printf 'ROOT=%q\n' "$ROOT"
    printf 'TEST_HOME=%q\n' "$TEST_HOME"
    printf 'TEST_PROJECT=%q\n' "$TEST_PROJECT"
    printf 'BIN_DIR=%q\n' "$BIN_DIR"
    printf 'REAL_HOME=%q\n' "$REAL_HOME"
    printf 'TEST_SHELL_NAME=%q\n' "$TEST_SHELL_NAME"
    printf 'TEST_SHELL_PATH=%q\n' "$TEST_SHELL_PATH"
  } > "$STATE_FILE"
}

state_exists() {
  load_state && [ -d "${ROOT:-}" ] && [ -x "${BIN_DIR:-}/avm" ]
}

ensure_not_existing() {
  if state_exists; then
    printf 'Test env already exists:\n'
    print_status
    printf '\nRun delete first if you want a fresh one.\n'
    exit 0
  fi
}

copy_if_present() {
  local src="$1"
  local dst="$2"
  if [ -f "$src" ]; then
    mkdir -p "$(dirname "$dst")"
    cp "$src" "$dst"
    chmod 600 "$dst" || true
    printf 'copied %s -> %s\n' "$src" "$dst"
  fi
}

install_filesystem_mcp() {
  mkdir -p "$TEST_HOME/.avm/registry/mcps"
  cat > "$TEST_HOME/.avm/registry/mcps/fs.yaml" <<'YAML'
name: fs
kind: mcp
server:
  type: stdio
  command: npx
  args:
    - -y
    - "@modelcontextprotocol/server-filesystem"
    - /tmp
YAML
}

install_superpowers_skills() {
  local checkout="$ROOT/superpowers"
  local skills_dir="$checkout/skills"
  local registry="$TEST_HOME/.avm/registry/skills"
  local names=()

  if ! command -v git >/dev/null 2>&1; then
    printf 'warning: git not found; skipping Superpowers skills install\n' >&2
    return 0
  fi

  printf 'Installing Superpowers skills from %s (%s)...\n' "$SUPERPOWERS_REPO" "$SUPERPOWERS_REF"
  if ! git clone --depth 1 --branch "$SUPERPOWERS_REF" "$SUPERPOWERS_REPO" "$checkout" >/dev/null 2>&1; then
    printf 'warning: could not clone Superpowers skills; continuing without them\n' >&2
    return 0
  fi

  if [ ! -d "$skills_dir" ]; then
    printf 'warning: Superpowers checkout has no skills directory; continuing without skills\n' >&2
    return 0
  fi

  mkdir -p "$registry"
  while IFS= read -r skill_file; do
    local name
    name="$(basename "$(dirname "$skill_file")")"
    case "$name" in
      ''|.*|*/*)
        continue
        ;;
    esac
    mkdir -p "$registry/$name"
    cp "$skill_file" "$registry/$name/SKILL.md"
    cat > "$registry/$name/meta.yaml" <<EOF
name: $name
kind: skill
description: Superpowers skill imported for AVM runtime-home testing.
source: $SUPERPOWERS_REPO
source_url: $SUPERPOWERS_REPO
tags:
  - superpowers
runtime_support:
  claude-code: native
  codex: native
EOF
    names+=("$name")
  done < <(find "$skills_dir" -mindepth 2 -maxdepth 2 -name SKILL.md | sort)

  if [ "${#names[@]}" -eq 0 ]; then
    return 0
  fi
  local joined
  joined="$(IFS=,; printf '%s' "${names[*]}")"
  printf '%s' "$joined" > "$ROOT/superpowers-skill-list"
  printf 'Installed Superpowers skills: %s\n' "$joined"
}

agent_create_args() {
  local runtime="$1"
  local model="$2"
  local name="$3"
  shift 3

  local args=(agent create "$name" --runtime "$runtime" --model "$model" --mcps fs)
  if [ -s "$ROOT/superpowers-skill-list" ]; then
    args+=(--skills "$(cat "$ROOT/superpowers-skill-list")")
  fi
  printf '%s\0' "${args[@]}"
}

detect_test_shell() {
  local requested="${AVM_TEST_SHELL:-${SHELL:-}}"
  local name path

  if [ -n "$requested" ]; then
    name="$(basename "$requested")"
  else
    name=""
  fi

  case "$name" in
    zsh|bash)
      path="$(command -v "$name" 2>/dev/null || true)"
      if [ -z "$path" ] && [ -x "$requested" ]; then
        path="$requested"
      fi
      ;;
    *)
      if path="$(command -v zsh 2>/dev/null)"; then
        name="zsh"
      elif path="$(command -v bash 2>/dev/null)"; then
        name="bash"
      else
        printf 'No supported shell found; need zsh or bash.\n' >&2
        exit 1
      fi
      ;;
  esac

  if [ -z "$path" ]; then
    printf 'Requested shell %q was not found on PATH.\n' "$name" >&2
    exit 1
  fi

  TEST_SHELL_NAME="$name"
  TEST_SHELL_PATH="$path"
}

write_shell_rc() {
  local script_path="$SCRIPT_DIR/avm-runtime-home-test-env.sh"
  local skills
  if [ -s "$ROOT/superpowers-skill-list" ]; then
    skills="$(cat "$ROOT/superpowers-skill-list")"
  else
    skills="none"
  fi

  cat > "$TEST_HOME/.avm-test-shell-common.sh" <<EOF
export AVM_TEST_ROOT=$(shell_quote "$ROOT")
export AVM_REAL_HOME=$(shell_quote "$REAL_HOME")
export AVM_BIN=$(shell_quote "$BIN_DIR/avm")
export PATH=$(shell_quote "$BIN_DIR"):\$PATH
cd $(shell_quote "$TEST_PROJECT")

avm-activate() {
  eval "\$(avm activate --kind env "\${1:-coding}")"
}

avm-delete-test-env() {
  $(shell_quote "$script_path") delete
}

avm-activate coding

printf '\\nAVM runtime-home test shell\\n'
printf '  shell=%s\\n' $(shell_quote "$TEST_SHELL_NAME")
printf '  HOME=%s\\n' "\$HOME"
printf '  project=%s\\n' "$(shell_quote "$TEST_PROJECT")"
printf '  CODEX_HOME=%s\\n' "\${CODEX_HOME:-}"
printf '  CLAUDE_CONFIG_DIR=%s\\n' "\${CLAUDE_CONFIG_DIR:-}"
printf '  OPENCODE_CONFIG=%s\\n' "\${OPENCODE_CONFIG:-}"
printf '  OPENCODE_CONFIG_DIR=%s\\n' "\${OPENCODE_CONFIG_DIR:-}"
if [ -n "\${ANTHROPIC_API_KEY:-}" ]; then
  printf '  ANTHROPIC_API_KEY=set\\n'
else
  printf '  ANTHROPIC_API_KEY=unset\\n'
fi
if [ -n "\${ANTHROPIC_AUTH_TOKEN:-}" ]; then
  printf '  ANTHROPIC_AUTH_TOKEN=set\\n'
else
  printf '  ANTHROPIC_AUTH_TOKEN=unset\\n'
fi
if [ -n "\${ANTHROPIC_BASE_URL:-}" ]; then
  printf '  ANTHROPIC_BASE_URL=set\\n'
else
  printf '  ANTHROPIC_BASE_URL=unset\\n'
fi
printf '  skills=%s\\n' $(shell_quote "$skills")
printf '\\nTry:\\n'
printf '  codex\\n'
printf '  # then run /mcp inside Codex\\n'
printf '  codex exec --skip-git-repo-check --ephemeral "Reply exactly AVM_CODEX_OK."\\n'
printf '  claude agents\\n'
printf '  claude auth status --text\\n'
printf '  opencode\\n'
printf '  # claude will ask you to log in here if this temporary HOME has no Claude auth\\n'
printf '\\nCleanup from your normal shell:\\n'
printf '  %s delete\\n\\n' "$(shell_quote "$script_path")"
EOF

  cat > "$TEST_HOME/.zshrc" <<'EOF'
source "$HOME/.avm-test-shell-common.sh"
eval "$(avm shell init zsh)"
EOF

  cat > "$TEST_HOME/.bashrc" <<'EOF'
source "$HOME/.avm-test-shell-common.sh"
eval "$(avm shell init bash)"
EOF
}

create_env() {
  ensure_not_existing
  detect_test_shell

  REAL_HOME="${AVM_REAL_HOME:-$HOME}"
  ROOT="$(mktemp -d /tmp/avm-runtime-home-test.XXXXXX)"
  TEST_HOME="$ROOT/home"
  TEST_PROJECT="$ROOT/project"
  BIN_DIR="$ROOT/bin"

  mkdir -p "$TEST_HOME" "$TEST_PROJECT" "$BIN_DIR"
  printf '# AVM runtime-home test project\n' > "$TEST_PROJECT/README.md"

  printf 'Building AVM from %s...\n' "$REPO_ROOT"
  (cd "$REPO_ROOT" && go build -o "$BIN_DIR/avm" ./cmd/avm)

  copy_if_present "$REAL_HOME/.codex/auth.json" "$TEST_HOME/.codex/auth.json"
  copy_if_present "$REAL_HOME/.claude.json" "$TEST_HOME/.claude.json"
  copy_if_present "$REAL_HOME/.claude/.credentials.json" "$TEST_HOME/.claude/.credentials.json"

  HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" init >/dev/null
  install_filesystem_mcp
  install_superpowers_skills

  local codex_args=()
  local claude_args=()
  local opencode_args=()
  while IFS= read -r -d '' arg; do codex_args+=("$arg"); done < <(agent_create_args codex "$CODEX_MODEL" codex-agent)
  while IFS= read -r -d '' arg; do claude_args+=("$arg"); done < <(agent_create_args claude-code "$CLAUDE_MODEL" claude-agent)
  while IFS= read -r -d '' arg; do opencode_args+=("$arg"); done < <(agent_create_args opencode "$OPENCODE_MODEL" opencode-agent)
  HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" "${codex_args[@]}" >/dev/null
  HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" "${claude_args[@]}" >/dev/null
  HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" "${opencode_args[@]}" >/dev/null
  HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" env create coding --codex codex-agent --claude-code claude-agent --opencode opencode-agent >/dev/null

  write_shell_rc
  save_state

  printf 'Created AVM runtime-home test env.\n'
  print_status
}

enter_env() {
  if ! state_exists; then
    printf 'No test env exists. Creating one first.\n'
    create_env
  fi
  load_state
  TEST_SHELL_NAME="${TEST_SHELL_NAME:-zsh}"
  TEST_SHELL_PATH="${TEST_SHELL_PATH:-$(command -v "$TEST_SHELL_NAME" 2>/dev/null || true)}"
  if [ -z "$TEST_SHELL_PATH" ]; then
    printf 'Configured test shell %q is not available.\n' "$TEST_SHELL_NAME" >&2
    exit 1
  fi
  printf 'Entering test shell. Type exit to return.\n'
  case "$TEST_SHELL_NAME" in
    zsh)
      HOME="$TEST_HOME" ZDOTDIR="$TEST_HOME" AVM_REAL_HOME="$REAL_HOME" AVM_TEST_ROOT="$ROOT" PATH="$BIN_DIR:$PATH" "$TEST_SHELL_PATH" -i
      ;;
    bash)
      HOME="$TEST_HOME" AVM_REAL_HOME="$REAL_HOME" AVM_TEST_ROOT="$ROOT" PATH="$BIN_DIR:$PATH" "$TEST_SHELL_PATH" --rcfile "$TEST_HOME/.bashrc" -i
      ;;
    *)
      printf 'Unsupported test shell %q.\n' "$TEST_SHELL_NAME" >&2
      exit 1
      ;;
  esac
}

delete_env() {
  if ! load_state; then
    printf 'No test env state found.\n'
    return 0
  fi
  if [ -z "${ROOT:-}" ] || [ "${ROOT#/tmp/avm-runtime-home-test.}" = "$ROOT" ]; then
    printf 'Refusing to delete unexpected path: %s\n' "${ROOT:-}"
    exit 1
  fi
  rm -rf "$ROOT"
  rm -f "$STATE_FILE"
  rmdir "$STATE_DIR" 2>/dev/null || true
  printf 'Deleted AVM runtime-home test env: %s\n' "$ROOT"
}

print_status() {
  if ! load_state; then
    printf 'No test env state found.\n'
    return 1
  fi
  printf '  root:    %s\n' "$ROOT"
  printf '  HOME:    %s\n' "$TEST_HOME"
  printf '  project: %s\n' "$TEST_PROJECT"
  printf '  avm:     %s\n' "$BIN_DIR/avm"
  printf '  shell:   %s (%s)\n' "${TEST_SHELL_NAME:-unknown}" "${TEST_SHELL_PATH:-unknown}"
}

cmd="${1:-start}"
case "$cmd" in
  start)
    if ! state_exists; then
      create_env
    fi
    enter_env
    ;;
  create)
    create_env
    ;;
  enter)
    enter_env
    ;;
  status)
    print_status
    ;;
  delete|clean)
    delete_env
    ;;
  -h|--help|help)
    usage
    ;;
  *)
    usage
    exit 2
    ;;
esac
