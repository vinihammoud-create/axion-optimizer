#!/bin/bash
# Stop hook: the agent may only finish when the changed Go code is formatted,
# passes go vet and its packages' tests pass. Exit 2 sends the agent back to
# work; any other failure of this script lets it finish (the CI still checks).
cd "${OPENHANDS_PROJECT_DIR:-.}" || exit 0

GO_DIR="$HOME/sdk/go1.27.4/go/bin"
[ -x "$GO_DIR/go" ] && PATH="$GO_DIR:$PATH"
command -v go >/dev/null 2>&1 || exit 0
export GOFLAGS=-mod=mod GOPROXY=off GOSUMDB=off CGO_ENABLED=0 GOTOOLCHAIN=local

base=$(git merge-base HEAD origin/main 2>/dev/null) || exit 0
changed=$( (git diff --name-only "$base"; git ls-files --others --exclude-standard) | grep -E '^(internal|cmd)/.*\.go$' | sort -u)
[ -z "$changed" ] && exit 0

# After three refusals the agent is released, so a check that can never pass
# does not burn the conversation budget.
count_file="${TMPDIR:-/tmp}/axion-verificar-$(git rev-parse --abbrev-ref HEAD | tr '/' '_')"
count=$(cat "$count_file" 2>/dev/null || echo 0)
[ "$count" -ge 3 ] && exit 0

deny() {
  echo $((count + 1)) >"$count_file"
  reason=$(printf '%s' "$1" | tr '\n"\\' '  /' | cut -c1-700)
  echo "{\"decision\": \"deny\", \"reason\": \"$reason\"}"
  exit 2
}

existing=$(for f in $changed; do [ -f "$f" ] && echo "$f"; done)
if [ -n "$existing" ]; then
  unformatted=$(gofmt -l $existing 2>&1)
  [ -n "$unformatted" ] && deny "gofmt: arquivos sem formatar: $unformatted"
fi

pkgs=$(for f in $existing; do echo "./$(dirname "$f")/"; done | sort -u)
[ -z "$pkgs" ] && exit 0
out=$(go vet $pkgs 2>&1) || deny "go vet falhou: $out"
out=$(go test $pkgs 2>&1) || deny "go test falhou: $(echo "$out" | tail -n 15)"
exit 0
