FROM golang:1.21 as build

WORKDIR /go/src/app
COPY . .

RUN CGO_ENABLED=0 go build -o /go/bin/app ./example/main.go

FROM gcr.io/distroless/static-debian11
COPY --from=build /go/bin/app /
CMD ["/app"]