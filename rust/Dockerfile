FROM rust:latest as builder
WORKDIR /app
COPY Cargo.toml Cargo.lock ./ 
COPY src ./src
RUN cargo build --release

FROM debian:stable-slim
WORKDIR /app
COPY --from=builder /app/target/release/gateway-watcher .
CMD ["./gateway-watcher"]
