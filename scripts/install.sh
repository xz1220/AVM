#!/usr/bin/env sh
set -eu

repo="${AVM_REPO:-xz1220/AVM}"
version="${AVM_VERSION:-latest}"
install_dir="${AVM_INSTALL_DIR:-$HOME/.local/bin}"
source_dir="${AVM_INSTALL_SOURCE_DIR:-}"
shell_integration="${AVM_INSTALL_SHELL_INTEGRATION:-1}"
install_ui="${AVM_INSTALL_UI:-1}"
skip_setup="${AVM_SKIP_SETUP:-${AVM_SKIP_INIT:-0}}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'avm install: required command not found: %s\n' "$1" >&2
    exit 1
  fi
}

download() {
  url="$1"
  dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$dest"
  else
    printf 'avm install: curl or wget is required\n' >&2
    exit 1
  fi
}

shell_quote() {
  printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
}

sha256_check() {
  checksums="$1"
  artifact="$2"
  artifact_name="$(basename "$artifact")"
  expected="$(awk -v name="$artifact_name" '$2 == name { print $1 }' "$checksums")"
  if [ -z "$expected" ]; then
    printf 'avm install: checksum for %s not found\n' "$artifact_name" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$artifact" | awk '{ print $1 }')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$artifact" | awk '{ print $1 }')"
  else
    printf 'avm install: sha256sum or shasum is required\n' >&2
    exit 1
  fi
  if [ "$actual" != "$expected" ]; then
    printf 'avm install: checksum mismatch for %s\n' "$artifact_name" >&2
    exit 1
  fi
}

check_node_for_ui() {
  if [ "$install_ui" = "0" ]; then
    return 0
  fi
  if ! command -v node >/dev/null 2>&1; then
    printf 'avm install: avm-ui requires Node.js >= 22 (set AVM_INSTALL_UI=0 for CLI-only install)\n' >&2
    exit 1
  fi
  node_major="$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || printf '0')"
  if [ "$node_major" -lt 22 ] 2>/dev/null; then
    printf 'avm install: avm-ui requires Node.js >= 22 (found %s; set AVM_INSTALL_UI=0 for CLI-only install)\n' "$(node --version 2>/dev/null || printf unknown)" >&2
    exit 1
  fi
}

case "$(uname -s)" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *)
    printf 'avm install: unsupported OS: %s\n' "$(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64) arch="x86_64" ;;
  *)
    printf 'avm install: unsupported architecture: %s\n' "$(uname -m)" >&2
    exit 1
    ;;
esac

tmp="${TMPDIR:-/tmp}/avm-install.$$"
mkdir -p "$tmp"
trap 'rm -rf "$tmp"' EXIT INT TERM

check_node_for_ui

if [ -n "$source_dir" ]; then
  need_cmd make
  need_cmd go
  build_home="${AVM_BUILD_HOME:-${AVM_REAL_HOME:-}}"
  if [ -n "$build_home" ]; then
    (cd "$source_dir" && HOME="$build_home" make build BIN="$tmp/avm")
  else
    (cd "$source_dir" && make build BIN="$tmp/avm")
  fi
  if [ "$install_ui" != "0" ]; then
    need_cmd npm
    (cd "$source_dir/ui" && npm ci && npm run typecheck && npm run build)
    cp "$source_dir/ui/dist/avm-ui.js" "$tmp/avm-ui"
    chmod +x "$tmp/avm-ui"
  fi
else
  need_cmd tar
  asset="avm_${os}_${arch}.tar.gz"
  if [ "$version" = "latest" ]; then
    base="https://github.com/$repo/releases/latest/download"
  else
    base="https://github.com/$repo/releases/download/$version"
  fi

  download "$base/$asset" "$tmp/$asset"
  download "$base/checksums.txt" "$tmp/checksums.txt"
  sha256_check "$tmp/checksums.txt" "$tmp/$asset"

  tar -xzf "$tmp/$asset" -C "$tmp"
fi

mkdir -p "$install_dir"
install "$tmp/avm" "$install_dir/avm"
printf 'installed avm to %s/avm\n' "$install_dir"

if [ "$install_ui" != "0" ]; then
  if [ ! -f "$tmp/avm-ui" ]; then
    printf 'avm install: release artifact did not include avm-ui (set AVM_INSTALL_UI=0 for CLI-only install)\n' >&2
    exit 1
  fi
  install "$tmp/avm-ui" "$install_dir/avm-ui"
  printf 'installed avm-ui to %s/avm-ui\n' "$install_dir"
fi

install_shell_integration() {
  shell_name="${AVM_INSTALL_SHELL:-}"
  if [ -z "$shell_name" ]; then
    shell_name="$(basename "${SHELL:-}")"
  fi

  case "$shell_name" in
    zsh)
      rc="${AVM_INSTALL_SHELL_RC:-$HOME/.zshrc}"
      ;;
    bash)
      rc="${AVM_INSTALL_SHELL_RC:-$HOME/.bashrc}"
      ;;
    fish)
      rc="${AVM_INSTALL_SHELL_RC:-$HOME/.config/fish/config.fish}"
      ;;
    *)
      printf 'avm install: shell integration skipped for unsupported shell: %s\n' "${shell_name:-unknown}" >&2
      printf 'add %s to PATH and run: avm shell install --shell zsh\n' "$install_dir" >&2
      return 0
      ;;
  esac

  "$install_dir/avm" shell install --shell "$shell_name" >/dev/null
  completion_path="${AVM_HOME:-$HOME/.avm}/shell/avm-completion.$shell_name"

  mkdir -p "$(dirname "$rc")"
  touch "$rc"
  if grep -q 'avm shell integration' "$rc" 2>/dev/null; then
    printf 'shell integration already present in %s\n' "$rc"
    return 0
  fi

  quoted_install_dir="$(shell_quote "$install_dir")"
  quoted_completion_path="$(shell_quote "$completion_path")"
  {
    printf '\n# >>> avm shell integration >>>\n'
    if [ "$shell_name" = "fish" ]; then
      printf 'set -l avm_install_dir %s\n' "$quoted_install_dir"
      printf 'if not contains -- $avm_install_dir $PATH\n'
      printf '    set -gx PATH $avm_install_dir $PATH\n'
      printf 'end\n'
      printf 'set -l avm_completion_path %s\n' "$quoted_completion_path"
      printf 'if test -f $avm_completion_path\n'
      printf '    source $avm_completion_path\n'
      printf 'end\n'
    else
      printf 'AVM_INSTALL_DIR=%s\n' "$quoted_install_dir"
      printf 'case ":$PATH:" in\n'
      printf '  *":$AVM_INSTALL_DIR:"*) ;;\n'
      printf '  *) export PATH="$AVM_INSTALL_DIR:$PATH" ;;\n'
      printf 'esac\n'
      printf 'unset AVM_INSTALL_DIR\n'
      printf 'if [ -f %s ]; then\n' "$quoted_completion_path"
      printf '  . %s\n' "$quoted_completion_path"
      printf 'fi\n'
    fi
    printf '# <<< avm shell integration <<<\n'
  } >> "$rc"

  printf 'installed shell integration to %s\n' "$rc"
  printf 'restart your shell or run: . %s\n' "$rc"
}

if [ "$skip_setup" != "1" ]; then
  "$install_dir/avm" setup ${AVM_SETUP_ARGS:-}
else
  printf 'skipped AVM setup\n'
fi

if [ "$shell_integration" != "0" ]; then
  install_shell_integration
else
  case ":$PATH:" in
    *":$install_dir:"*) ;;
    *) printf 'add %s to PATH to run avm from any shell\n' "$install_dir" ;;
  esac
fi

if [ "$skip_setup" = "1" ]; then
  printf 'next:\n'
  printf '  avm setup\n'
  if [ "$install_ui" != "0" ]; then
    printf '  avm-ui\n'
  fi
  printf '  avm agent create --name backend-coder --runtime <runtime>\n'
  printf '  avm run backend-coder\n'
fi
