# dockerfile for go web app (go version 1.19)

FROM golang:1.19

WORKDIR /go/src/app

COPY . .

RUN go get -d -v ./...

RUN go install -v ./...

RUN go build

EXPOSE 8080

CMD ["environment-test"]
