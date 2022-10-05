FROM golang:1.19.1-alpine AS build

WORKDIR /app

COPY . .

RUN apk add --update git ca-certificates gcc musl-dev && \
  go mod download && \
  GO111MODULE=on CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -v -o run && \
  chmod +x run && \
  mkdir /data

FROM alpine:3.16.2

COPY --from=build /data /data
COPY --from=build /app/run /app/run
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8080

ENTRYPOINT ["/app/run"]