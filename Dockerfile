FROM golang:1.19-alpine as build

RUN apk add --update git

WORKDIR /app

COPY . .

RUN go mod download && \
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -v -installsuffix cgo -o run

FROM scratch

COPY --from=build /app/run run

EXPOSE 8080

ENTRYPOINT ["./run"]