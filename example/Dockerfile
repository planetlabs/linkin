FROM golang:1.10-alpine3.7 AS build

RUN apk update && apk add git

WORKDIR /go/src/github.com/planetlabs/example
COPY . .

RUN go get -u github.com/Masterminds/glide
RUN glide install
RUN go build -o /example example.go

FROM alpine:3.7

RUN apk update && apk add ca-certificates
COPY --from=build /example /example