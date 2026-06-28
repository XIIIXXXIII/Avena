FROM gcc:latest AS builder
WORKDIR /app
COPY . .
RUN g++ -std=c++17 -O2 -Wall -o string-filter main.cpp

FROM debian:stable-slim
WORKDIR /app
COPY --from=builder /app/string-filter .
CMD ["./string-filter"]
