#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)"
DEFAULT_STATE_DIR="$(git -C "$REPO_ROOT" rev-parse --git-path avm-runtime-home-test-env 2>/dev/null || printf '%s/.avm-runtime-home-test-env' "$REPO_ROOT")"
STATE_DIR="${AVM_TEST_STATE_DIR:-$DEFAULT_STATE_DIR}"
STATE_FILE="$STATE_DIR/state"

CODEX_MODEL="${AVM_TEST_CODEX_MODEL:-gpt-5.4-mini}"
CLAUDE_MODEL="${AVM_TEST_CLAUDE_MODEL:-sonnet}"
OPENCODE_MODEL="${AVM_TEST_OPENCODE_MODEL:-anthropic/claude-sonnet-4-5}"
SUPERPOWERS_REPO="${AVM_TEST_SUPERPOWERS_REPO:-https://github.com/obra/superpowers.git}"
SUPERPOWERS_REF="${AVM_TEST_SUPERPOWERS_REF:-main}"
COPY_RUNTIME_CONFIG="${AVM_TEST_COPY_RUNTIME_CONFIG:-0}"

usage() {
  cat <<'EOF'
Usage:
  scripts/dev/avm-runtime-home-test-env.sh start   # create if needed, then enter clean isolated shell
  scripts/dev/avm-runtime-home-test-env.sh create  # create clean isolated shell only; does not install avm
  scripts/dev/avm-runtime-home-test-env.sh seed    # opt in to demo AVM init/agents/env setup
  scripts/dev/avm-runtime-home-test-env.sh enter   # enter existing test shell
  scripts/dev/avm-runtime-home-test-env.sh status  # show current test env
  scripts/dev/avm-runtime-home-test-env.sh delete  # delete test env

Options:
  AVM_TEST_SHELL=zsh|bash             Override the shell used by start/enter.
  AVM_TEST_STATE_DIR=/path            Store script state outside the repo git dir.
  AVM_TEST_COPY_RUNTIME_CONFIG=1      Copy runtime config/auth snapshots into the clean HOME.
  AVM_TEST_CLAUDE_MODEL=sonnet        Override the seeded Claude Code agent model.
  AVM_TEST_OPENCODE_MODEL=...         Override the seeded OpenCode agent model.

The default environment intentionally starts without avm installed. Inside the
test shell, run the printed helper, then try:
  avm-install-local
  avm create
  avm use <agent-name>
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
  load_state && [ -d "${ROOT:-}" ] && [ -d "${TEST_HOME:-}" ] && [ -d "${TEST_PROJECT:-}" ]
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

copy_dir_if_present() {
  local src="$1"
  local dst="$2"
  if [ -d "$src" ]; then
    mkdir -p "$(dirname "$dst")"
    if cp -a "$src" "$dst"; then
      printf 'copied %s -> %s\n' "$src" "$dst"
    else
      printf 'warning: could not copy %s -> %s\n' "$src" "$dst" >&2
    fi
  fi
}

copy_runtime_config_snapshot() {
  if [ "$COPY_RUNTIME_CONFIG" = "0" ]; then
    printf 'runtime config snapshot disabled\n'
    return 0
  fi

  copy_dir_if_present "$REAL_HOME/.codex" "$TEST_HOME/.codex"
  copy_if_present "$REAL_HOME/.claude.json" "$TEST_HOME/.claude.json"
  copy_dir_if_present "$REAL_HOME/.claude" "$TEST_HOME/.claude"
  copy_dir_if_present "$REAL_HOME/.config/opencode" "$TEST_HOME/.config/opencode"
  copy_dir_if_present "$REAL_HOME/.cline" "$TEST_HOME/.cline"
}

auth_env_names() {
  cat <<'EOF'
ANTHROPIC_API_KEY
ANTHROPIC_AUTH_TOKEN
ANTHROPIC_BASE_URL
ANTHROPIC_BEDROCK_BASE_URL
ANTHROPIC_BEDROCK_SERVICE_TIER
ANTHROPIC_MODEL
ANTHROPIC_SMALL_FAST_MODEL
ANTHROPIC_VERTEX_PROJECT_ID
ANTHROPIC_VERTEX_REGION
CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS
CLAUDE_CODE_MAX_OUTPUT_TOKENS
CLAUDE_CODE_OAUTH_TOKEN
EOF
}

capture_real_shell_env() {
  local target="$ROOT/real-auth-shell.env"
  local capture_script="$ROOT/capture-auth-env.sh"
  {
    printf 'for __avm_name in'
    while IFS= read -r name; do
      [ -n "$name" ] || continue
      printf ' %s' "$name"
    done < <(auth_env_names)
    printf '; do\n'
    printf '  eval "__avm_value=\\${$__avm_name-}"\n'
    printf '  if [ -n "$__avm_value" ]; then\n'
    printf '    printf '"'"'%s=%s\\n'"'"' "$__avm_name" "$__avm_value"\n'
    printf '  fi\n'
    printf 'done\n'
  } > "$capture_script"
  : > "$target"
  case "$TEST_SHELL_NAME" in
    zsh)
      HOME="$REAL_HOME" ZDOTDIR="$REAL_HOME" "$TEST_SHELL_PATH" -ic ". $(shell_quote "$capture_script")" > "$target" 2>/dev/null || true
      ;;
    bash)
      HOME="$REAL_HOME" "$TEST_SHELL_PATH" -ic ". $(shell_quote "$capture_script")" > "$target" 2>/dev/null || true
      ;;
  esac
  rm -f "$capture_script"
  printf '%s' "$target"
}

env_value_from_dump() {
  local name="$1"
  local dump="$2"
  if [ -r "$dump" ]; then
    awk -v key="$name" 'index($0, key "=") == 1 { sub("^[^=]*=", ""); print; exit }' "$dump"
  fi
}

write_auth_env_snapshot() {
  local target="$TEST_HOME/.avm-test-auth-env.sh"
  : > "$target"
  if [ "$COPY_RUNTIME_CONFIG" = "0" ]; then
    return 0
  fi

  local shell_env_dump
  shell_env_dump="$(capture_real_shell_env)"
  local count=0
  while IFS= read -r name; do
    [ -n "$name" ] || continue
    local value="${!name:-}"
    if [ -z "$value" ]; then
      value="$(env_value_from_dump "$name" "$shell_env_dump")"
    fi
    if [ -n "$value" ]; then
      printf 'export %s=%q\n' "$name" "$value" >> "$target"
      count=$((count + 1))
    fi
  done < <(auth_env_names)

  chmod 600 "$target" || true
  if [ "$count" -gt 0 ]; then
    printf 'copied %s allowlisted auth env vars into isolated shell\n' "$count"
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
  local install_source install_script rc_source_cmd rc_path
  install_source="$(shell_quote "$REPO_ROOT")"
  install_script="$(shell_quote "$REPO_ROOT/scripts/install.sh")"
  case "$TEST_SHELL_NAME" in
    zsh)
      rc_source_cmd="source"
      rc_path="$(shell_quote "$TEST_HOME/.zshrc")"
      ;;
    bash)
      rc_source_cmd="."
      rc_path="$(shell_quote "$TEST_HOME/.bashrc")"
      ;;
    *)
      rc_source_cmd="restart"
      rc_path="this shell"
      ;;
  esac

  cat > "$TEST_HOME/.avm-test-shell-common.sh" <<EOF
export AVM_TEST_ROOT=$(shell_quote "$ROOT")
export AVM_REAL_HOME=$(shell_quote "$REAL_HOME")
export SHELL=$(shell_quote "$TEST_SHELL_PATH")
if [ -r "\$HOME/.avm-test-auth-env.sh" ]; then
  source "\$HOME/.avm-test-auth-env.sh"
fi
cd $(shell_quote "$TEST_PROJECT")

avm-delete-test-env() {
  $(shell_quote "$script_path") delete
}

avm-install-local() {
  AVM_INSTALL_SOURCE_DIR=$(shell_quote "$REPO_ROOT") $(shell_quote "$REPO_ROOT/scripts/install.sh") "\$@"
  case $(shell_quote "$TEST_SHELL_NAME") in
    zsh)
      source "\$HOME/.zshrc" >/dev/null
      ;;
    bash)
      . "\$HOME/.bashrc" >/dev/null
      ;;
  esac
  printf 'loaded AVM shell integration for %s\\n' $(shell_quote "$TEST_SHELL_NAME")
}

printf '\\nAVM isolated test shell\\n'
printf '  shell=%s\\n' $(shell_quote "$TEST_SHELL_NAME")
printf '  SHELL=%s\\n' "\$SHELL"
printf '  HOME=%s\\n' "\$HOME"
printf '  real_HOME=%s\\n' "\$AVM_REAL_HOME"
printf '  project=%s\\n' "$(shell_quote "$TEST_PROJECT")"
if command -v avm >/dev/null 2>&1; then
  printf '  avm=%s\\n' "\$(command -v avm)"
else
  printf '  avm=not installed\\n'
fi
printf '  AVM_HOME=%s\\n' "\${AVM_HOME:-\$HOME/.avm}"
printf '  AVM_ACTIVE=%s\\n' "\${AVM_ACTIVE:-unset}"
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
printf '\\nFirst-user install path to try:\\n'
printf '  avm-install-local\\n'
printf '  avm create\\n'
printf '  avm use <agent-name>\\n'
printf '\\nEquivalent explicit install commands:\\n'
printf '  AVM_INSTALL_SOURCE_DIR=%s %s\\n' "$install_source" "$install_script"
printf '  %s %s\\n' "$rc_source_cmd" "$rc_path"
printf '\\nOptional seeded demo setup:\\n'
printf '  %s seed\\n' "$(shell_quote "$script_path")"
printf '\\nCleanup from your normal shell:\\n'
printf '  %s delete\\n\\n' "$(shell_quote "$script_path")"
EOF

  cat > "$TEST_HOME/.zshrc" <<'EOF'
source "$HOME/.avm-test-shell-common.sh"
EOF

  cat > "$TEST_HOME/.bashrc" <<'EOF'
source "$HOME/.avm-test-shell-common.sh"
EOF
}

create_env() {
  ensure_not_existing
  detect_test_shell

  REAL_HOME="${AVM_REAL_HOME:-$HOME}"
  ROOT="$(mktemp -d /tmp/avm-runtime-home-test.XXXXXX)"
  TEST_HOME="$ROOT/home"
  TEST_PROJECT="$ROOT/project"
  BIN_DIR="$TEST_HOME/.local/bin"

  mkdir -p "$TEST_HOME" "$TEST_PROJECT"
  printf '# AVM runtime-home test project\n' > "$TEST_PROJECT/README.md"

  copy_runtime_config_snapshot
  write_auth_env_snapshot

  write_shell_rc
  save_state

  printf 'Created clean AVM isolated test shell. AVM is not installed yet.\n'
  print_status
}

seed_env() {
  if ! state_exists; then
    printf 'No test env exists. Creating one first.\n'
    create_env
  fi
  load_state

  if [ ! -x "$BIN_DIR/avm" ]; then
    printf 'Installing AVM into the isolated HOME first...\n'
    HOME="$TEST_HOME" \
      SHELL="$TEST_SHELL_PATH" \
      AVM_INSTALL_SOURCE_DIR="$REPO_ROOT" \
      AVM_INSTALL_DIR="$BIN_DIR" \
      AVM_INSTALL_SHELL="$TEST_SHELL_NAME" \
      "$REPO_ROOT/scripts/install.sh" >/dev/null
  fi

  HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" init >/dev/null
  install_filesystem_mcp
  install_superpowers_skills

  local codex_args=()
  local claude_args=()
  local opencode_args=()
  while IFS= read -r -d '' arg; do codex_args+=("$arg"); done < <(agent_create_args codex "$CODEX_MODEL" codex-agent)
  while IFS= read -r -d '' arg; do claude_args+=("$arg"); done < <(agent_create_args claude-code "$CLAUDE_MODEL" claude-agent)
  while IFS= read -r -d '' arg; do opencode_args+=("$arg"); done < <(agent_create_args opencode "$OPENCODE_MODEL" opencode-agent)
  [ -f "$TEST_HOME/.avm/agents/codex-agent.yaml" ] || HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" "${codex_args[@]}" >/dev/null
  [ -f "$TEST_HOME/.avm/agents/claude-agent.yaml" ] || HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" "${claude_args[@]}" >/dev/null
  [ -f "$TEST_HOME/.avm/agents/opencode-agent.yaml" ] || HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" "${opencode_args[@]}" >/dev/null
  [ -f "$TEST_HOME/.avm/envs/coding.yaml" ] || HOME="$TEST_HOME" PATH="$BIN_DIR:$PATH" "$BIN_DIR/avm" env create coding --codex codex-agent --claude-code claude-agent --opencode opencode-agent >/dev/null

  printf 'Seeded AVM demo agents and env in the isolated test shell.\n'
  printf 'Activate with:\n'
  printf '  eval "$(avm activate --kind env coding)"\n'
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
      HOME="$TEST_HOME" ZDOTDIR="$TEST_HOME" AVM_REAL_HOME="$REAL_HOME" AVM_TEST_ROOT="$ROOT" "$TEST_SHELL_PATH" -i
      ;;
    bash)
      HOME="$TEST_HOME" AVM_REAL_HOME="$REAL_HOME" AVM_TEST_ROOT="$ROOT" "$TEST_SHELL_PATH" --rcfile "$TEST_HOME/.bashrc" -i
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
  if [ -x "$BIN_DIR/avm" ]; then
    printf '  avm:     %s\n' "$BIN_DIR/avm"
  else
    printf '  avm:     not installed (target %s/avm)\n' "$BIN_DIR"
  fi
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
  seed)
    seed_env
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
