FROM golang:1.17-alpine3.14 AS build
COPY . /go/src/github.com/dollarshaveclub/thermite/
WORKDIR /go/src/github.com/dollarshaveclub/thermite/
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /bin/thermite
FROM alpine:3.14
COPY --from=build /bin/thermite /bin/thermite
ENTRYPOINT ["/bin/thermite"]