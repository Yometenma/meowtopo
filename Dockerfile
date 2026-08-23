FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/moetopo ./cmd/moetopo

FROM alpine:3.21
RUN addgroup -S moetopo && adduser -S -G moetopo -h /app moetopo && mkdir -p /data && chown moetopo:moetopo /data
COPY --from=build /out/moetopo /usr/local/bin/moetopo
USER moetopo
VOLUME ["/data"]
EXPOSE 8088
ENV MOETOPO_DATA_DIR=/data MOETOPO_HTTP_ADDR=0.0.0.0:8088
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD wget -qO- http://127.0.0.1:8088/api/health || exit 1
ENTRYPOINT ["/usr/local/bin/moetopo"]
