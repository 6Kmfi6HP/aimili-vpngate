# syntax=docker/dockerfile:1

FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/aimilivpn ./cmd/aimilivpn \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/aimilivpnctl ./cmd/aimilivpnctl

FROM debian:bookworm-slim AS runtime
ENV AIMILIVPN_CONTAINER=true \
    VPNGATE_DATA_DIR=/data \
    UI_HOST=0.0.0.0 \
    LOCAL_PROXY_HOST=0.0.0.0
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates openvpn iproute2 iptables curl tini \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /opt/aimilivpn
COPY --from=build /out/aimilivpn /usr/local/bin/aimilivpn
COPY --from=build /out/aimilivpnctl /usr/local/bin/aimilivpnctl
COPY packaging/docker/entrypoint.sh /usr/local/bin/aimilivpn-entrypoint
RUN chmod +x /usr/local/bin/aimilivpn-entrypoint \
    && mkdir -p /data /opt/aimilivpn \
    && ln -sf /usr/local/bin/aimilivpnctl /usr/local/bin/ml
VOLUME ["/data"]
EXPOSE 8787 7928
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/aimilivpn-entrypoint"]
CMD ["serve"]
