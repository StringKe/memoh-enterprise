#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

make_fake_bin() {
  name="$1"
  cat > "$TMPDIR/$name" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF
  chmod +x "$TMPDIR/$name"
}

for tool in git containerd nerdctl buildctl docker podman; do
  make_fake_bin "$tool"
done

run_dry_run() {
  runtime="$1"
  output="$TMPDIR/${runtime}.out"
  PATH="$TMPDIR:$PATH" MEMOH_ALLOW_ROOT_INSTALL=true \
    sh "$ROOT/scripts/install.sh" --yes --dry-run --runtime "$runtime" --container-backend "$runtime" --version latest \
    > "$output"

  grep -q "deployment runtime: ${runtime}" "$output"
  grep -q "image registry: ghcr.io/stringke" "$output"
  grep -q "server:              26810" "$output"
  grep -q "worker:              26816" "$output"
  grep -q "postgres:            26817" "$output"
  grep -q "qdrant:              26818" "$output"
  if grep -Eiq 'docker hub|dockerhub|npmjs|registry\.npmjs\.org|china|cn mirror' "$output"; then
    echo "dry-run output contains removed registry or regional optimization text" >&2
    cat "$output" >&2
    exit 1
  fi
}

run_dry_run containerd
run_dry_run docker
run_dry_run podman

echo "install dry-run tests passed"
