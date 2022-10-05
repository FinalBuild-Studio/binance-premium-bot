FROM golang:1.19.1-alpine as build

RUN apk add --update git ca-certificates

WORKDIR /app

COPY . .

RUN go mod download && \
  CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -a -v -installsuffix cgo -o run

FROM scratch

COPY --from=build /app/run run
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8080

ENTRYPOINT ["./run"]