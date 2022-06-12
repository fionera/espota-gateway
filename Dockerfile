FROM golang:1.18 as build-env

WORKDIR /go/src/app
COPY *.go /go/src/app/

RUN go mod init
RUN go get -d -v ./...
RUN go vet -v

RUN CGO_ENABLED=0 go build -o /go/bin/app

FROM gcr.io/distroless/static

COPY --from=build-env /go/bin/app /
CMD ["/app"]