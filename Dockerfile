FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/meowtopo ./cmd/meowtopo

FROM alpine:3.21
RUN apk add --no-cache su-exec \
    && addgroup -S -g 10001 meowtopo \
    && adduser -S -D -H -u 10001 -G meowtopo meowtopo \
    && mkdir -p /data \
    && chown meowtopo:meowtopo /data
COPY --from=build /out/meowtopo /usr/local/bin/meowtopo
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh
VOLUME ["/data"]
EXPOSE 8088
ENV MEOWTOPO_DATA_DIR=/data MEOWTOPO_HTTP_ADDR=0.0.0.0:8088
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8088/api/health || exit 1
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
