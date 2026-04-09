FROM golang:1.26-alpine AS builder

RUN --mount=type=cache,target=/var/cache/apk \
    apk add git ca-certificates gcc musl-dev

WORKDIR /build

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=bind,source=go.mod,target=go.mod \
    --mount=type=bind,source=go.sum,target=go.sum \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -tags goolm -o /mautrix-boosty ./cmd/mautrix-boosty/

FROM alpine:3

RUN --mount=type=cache,target=/var/cache/apk \
    apk add ca-certificates su-exec

COPY --from=builder /mautrix-boosty /usr/bin/mautrix-boosty

ENV UID=1337 GID=1337
VOLUME /data

CMD ["/usr/bin/mautrix-boosty", "-c", "/data/config.yaml"]
