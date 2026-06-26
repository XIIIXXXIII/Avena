FROM golang:1.22-alpine as builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o command-router .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/command-router .
CMD ["./command-router"]
