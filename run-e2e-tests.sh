#!/usr/bin/env bash

set -e

log_and_run() {
  echo "> $@"
  "$@"
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

  # -slim replaces -no-plugins from 2.0; older minors were only ever published as -no-plugins.
  case "$KESTRA_VERSION" in
    v0.*|v1.*) export KESTRA_IMAGE_SUFFIX="no-plugins" ;;
    *) export KESTRA_IMAGE_SUFFIX="slim" ;;
  esac

  echo "start Kestra container"
  log_and_run docker compose -f docker-compose-ci.yml down

  export KESTRA_VERSION=$KESTRA_VERSION
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
