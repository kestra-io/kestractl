# kestractl - Alpine variant (default).
#
# Ships a shell plus git/curl on purpose: the primary use case is CI/CD jobs
# (GitLab CI, GitHub Actions, ...) that chain kestractl with other commands in a
# multi-command script, which a distroless image cannot host. Use the `-static`
# variant when you only ever exec kestractl itself.
#
# The binary is NOT compiled here: GoReleaser builds the per-platform, statically
# linked binary and lays the build context out as <os>/<arch>/kestractl, hence the
# $TARGETPLATFORM prefix. Same pattern as kestra-io/kestra: build elsewhere, COPY
# here.
FROM alpine:3

ARG TARGETPLATFORM

RUN apk add --no-cache ca-certificates git curl bash \
    && adduser -D -u 1000 kestractl

COPY $TARGETPLATFORM/kestractl /usr/local/bin/kestractl

USER kestractl
WORKDIR /home/kestractl

ENTRYPOINT ["kestractl"]
CMD ["--help"]
