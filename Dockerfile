FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/matrix-mileage-bot .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 bot
USER bot
WORKDIR /data
COPY --from=build /out/matrix-mileage-bot /usr/local/bin/matrix-mileage-bot
ENTRYPOINT ["/usr/local/bin/matrix-mileage-bot"]
CMD ["-config", "/data/config.yaml"]
