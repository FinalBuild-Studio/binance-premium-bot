FROM golang:1.19.1-alpine as build

RUN apk add --update git ca-certificates gcc g++

WORKDIR /app

COPY . .

RUN go mod download && \
  go build -v -o run

FROM scratch

COPY --from=build /app/run run
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

EXPOSE 8080

ENTRYPOINT ["./run"]