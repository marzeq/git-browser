# syntax=docker/dockerfile:1.7

FROM golang:1.26.4-bookworm AS git-browser-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/git-browser \
    .

FROM debian:bookworm-slim AS gitolite-source

ARG GITOLITE_COMMIT=9d3d03d7b82e71e1bf4b9544876386b38f87f273
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates git \
    && rm -rf /var/lib/apt/lists/* \
    && git clone --filter=blob:none https://github.com/sitaramc/gitolite.git /opt/gitolite \
    && git -C /opt/gitolite checkout --detach "${GITOLITE_COMMIT}" \
    && test "$(git -C /opt/gitolite rev-parse HEAD)" = "${GITOLITE_COMMIT}" \
    && rm -rf /opt/gitolite/.git

FROM debian:bookworm-slim

ARG GITOLITE_VERSION=3.6.13
LABEL org.opencontainers.image.title="git-browser + Gitolite" \
      org.opencontainers.image.description="Self-contained SSH Git hosting with Gitolite authorization and a read-only web browser" \
      org.opencontainers.image.version="${GITOLITE_VERSION}" \
      org.opencontainers.image.source="https://github.com/sitaramc/gitolite"

RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
        bash \
        ca-certificates \
        curl \
        git \
        openssh-server \
        passwd \
        perl \
        tini \
        util-linux \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 1000 git \
    && useradd --uid 1000 --gid git --home-dir /var/lib/gitolite --shell /bin/bash git \
    && passwd --delete git \
    && install -d -o git -g git -m 0750 /var/lib/gitolite \
    && install -d -o git -g git -m 0700 /var/lib/gitolite/.ssh \
    && install -d -m 0755 /run/sshd /run/git-browser-appliance

COPY --from=git-browser-builder /out/git-browser /usr/local/bin/git-browser
COPY --from=gitolite-source /opt/gitolite /opt/gitolite
COPY docker/entrypoint.sh /usr/local/sbin/git-browser-entrypoint
COPY docker/healthcheck.sh /usr/local/sbin/git-browser-healthcheck
COPY docker/sshd_config /etc/ssh/sshd_config

RUN chmod 0755 \
        /usr/local/bin/git-browser \
        /usr/local/sbin/git-browser-entrypoint \
        /usr/local/sbin/git-browser-healthcheck \
    && test -x /opt/gitolite/install \
    && test -f /opt/gitolite/src/gitolite-shell

ENV GITOLITE_HOME=/var/lib/gitolite \
    GITOLITE_ADMIN_NAME=admin \
    GITOLITE_ADMIN_KEY_FILE=/run/secrets/gitolite_admin_key \
    GIT_BROWSER_LISTEN=0.0.0.0:8080

VOLUME ["/var/lib/gitolite"]
EXPOSE 22 8080
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD ["/usr/local/sbin/git-browser-healthcheck"]

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/sbin/git-browser-entrypoint"]
