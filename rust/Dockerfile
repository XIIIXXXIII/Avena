FROM rust:latest as builder
WORKDIR /app
COPY Cargo.toml ./
# Create a dummy source file to build dependencies and cache them
RUN mkdir src && echo "fn main() {}" > src/main.rs && cargo build --release && rm -rf src
COPY src ./src
RUN touch src/main.rs && cargo build --release

FROM debian:stable-slim
WORKDIR /app
RUN apt-get update && apt-get install -y libssl-dev ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/target/release/gateway-watcher .
CMD ["./gateway-watcher"]
