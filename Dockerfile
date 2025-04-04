# Use buildx-compatible base images
FROM --platform=$TARGETPLATFORM rust:slim AS rbuilder
WORKDIR /backend
COPY backend .
# Install build dependencies
RUN apt-get update && apt-get install -y pkg-config libssl-dev
RUN cargo install sqlx-cli
RUN sqlx db create
RUN sqlx migrate run
RUN cargo build --release
RUN strip target/release/watch-together

FROM --platform=$TARGETPLATFORM node:20-slim AS jbuilder
WORKDIR /frontend
COPY frontend .
RUN npm install
RUN npm run build

FROM --platform=$TARGETPLATFORM debian:bookworm-slim AS release
WORKDIR /app
# Install runtime dependencies
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=rbuilder /backend/target/release/watch-together .
COPY --from=jbuilder /frontend/dist/ dist/
EXPOSE 8080
CMD ["./watch-together", "--tracing-level", "INFO", "--run-migrations"]
