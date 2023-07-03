FROM golang:1.19-buster AS build

WORKDIR /app

COPY . .
RUN go mod download


RUN go build

CMD [ "./mariadb-go" ]