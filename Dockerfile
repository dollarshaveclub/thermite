FROM golang:1.18-alpine3.15 AS build
COPY . /go/src/github.com/dollarshaveclub/thermite/
WORKDIR /go/src/github.com/dollarshaveclub/thermite/
RUN apk update && apk add git && CGO_ENABLED=0 go build -ldflags='-s -w' -o /bin/thermite
FROM alpine:3.15
COPY --from=build /bin/thermite /bin/thermite
ENTRYPOINT ["/bin/thermite"]