FROM golang:latest as builder

WORKDIR /app

COPY go.mod go.sum .gitignore ./

RUN go mod download

COPY main.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/app .

CMD ["./app"]
