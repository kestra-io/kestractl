#!/usr/bin/env bash

set -e

log_and_run() {
  echo "> $@"
  "$@"
}

# -slim replaces -no-plugins (kestra-io/kestra#16054, kestra-io/actions#204), but
# older/unreleased versions may not have published a -slim image yet. Probe the
# registry and fall back so CI doesn't break on those.
resolve_kestra_image_suffix() {
  local version="$1"
  local repo="europe-west1-docker.pkg.dev/kestra-host/docker/kestra-ee"

  if docker pull -q "${repo}:${version}-slim" >/dev/null 2>&1; then
    echo "-slim"
  else
    echo "no ${repo}:${version}-slim, falling back to -no-plugins" >&2
    echo "-no-plugins"
  fi
}

if [ $# -ge 1 ]; then
  versions="$1"
else
  versions=$(cat ./COMPATIBLE_KESTRA_VERSION.properties)
fi

echo "/n------------------------------------------------"
echo "Build Kestra CLI and test it with a docker Kestra instance"

for KESTRA_VERSION in $versions; do
  if [ -z "$KESTRA_VERSION" ]; then
    continue
  fi

  echo "docker KESTRA_VERSION used: $KESTRA_VERSION\n"

  export KESTRA_VERSION=$KESTRA_VERSION
  export KESTRA_IMAGE_SUFFIX=$(resolve_kestra_image_suffix "$KESTRA_VERSION")

  echo "start Kestra container"
  log_and_run docker compose -f docker-compose-ci.yml down
  log_and_run docker compose -f docker-compose-ci.yml up -d --wait || {
     echo "db Docker Compose failed. Dumping logs:";
     log_and_run docker compose -f docker-compose-ci.yml logs;
     exit 1;
  }

  echo "build"
  log_and_run sh -c 'go build -C ./e2e_tests ./...'

  echo "start tests"
  log_and_run sh -c 'go test -C ./e2e_tests ./...'

  echo "stop Kestra container"
  log_and_run docker compose -f docker-compose-ci.yml down
done
