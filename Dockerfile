FROM golang:1.19.1-alpine as build

WORKDIR /app

COPY . .

RUN apk add --update git ca-certificates gcc g++ musl-dev && \
  go mod download && \
  go build -o run && \
  mkdir /data

FROM scratch

COPY --from=build /data /data
COPY --from=build /app/run run
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8080

ENTRYPOINT ["./run"]