FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/meowtopo ./cmd/meowtopo

FROM alpine:3.21
RUN addgroup -S meowtopo && adduser -S -G meowtopo -h /app meowtopo && mkdir -p /data && chown meowtopo:meowtopo /data
COPY --from=build /out/meowtopo /usr/local/bin/meowtopo
USER meowtopo
VOLUME ["/data"]
EXPOSE 8088
ENV MEOWTOPO_DATA_DIR=/data MEOWTOPO_HTTP_ADDR=0.0.0.0:8088
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8088/api/health || exit 1
ENTRYPOINT ["/usr/local/bin/meowtopo"]
