FROM rust:latest AS builder
WORKDIR /app

# Кэшируем зависимости отдельным слоем
COPY Cargo.toml ./
RUN mkdir src && echo "fn main() {}" > src/main.rs \
    && cargo build --release \
    && rm -rf src

# Собираем реальный код
COPY src ./src
RUN touch src/main.rs && cargo build --release

FROM debian:stable-slim
WORKDIR /app
RUN apt-get update \
    && apt-get install -y ca-certificates libssl3 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/target/release/avena-executor .
CMD ["./avena-executor"]
