FROM gcc:latest as builder
WORKDIR /app
COPY . .
RUN g++ -o string-processor main.cpp

FROM debian:stable-slim
WORKDIR /app
COPY --from=builder /app/string-processor .
CMD ["./string-processor"]
