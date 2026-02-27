FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -o alertstoclaude .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/alertstoclaude /usr/local/bin/
EXPOSE 8080
ENTRYPOINT ["alertstoclaude"]
