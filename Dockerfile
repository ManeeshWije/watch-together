# Chef stage for dependency caching
FROM --platform=$TARGETPLATFORM lukemathwalker/cargo-chef:latest AS chef
WORKDIR /backend

# Planner stage
FROM --platform=$TARGETPLATFORM chef AS planner
COPY backend .
RUN cargo chef prepare --recipe-path recipe.json

# Builder stage for Rust
FROM --platform=$TARGETPLATFORM chef AS rbuilder
COPY --from=planner /backend/recipe.json recipe.json
# Install build dependencies
RUN apt-get update && apt-get install -y pkg-config libssl-dev
# Cook dependencies first (for better caching)
RUN cargo chef cook --release --recipe-path recipe.json
# Copy application code and build
COPY backend .
ENV SQLX_OFFLINE=true
RUN cargo build --release
RUN strip target/release/watch-together

# Builder stage for Node.js
FROM --platform=$TARGETPLATFORM node:20-slim AS jbuilder
WORKDIR /frontend
COPY frontend .
RUN npm install
RUN npm run build

# Final runtime stage
FROM --platform=$TARGETPLATFORM debian:bookworm-slim AS release
WORKDIR /app
# Install runtime dependencies
RUN apt-get update && apt-get install -y ca-certificates
COPY --from=rbuilder /backend/target/release/watch-together .
COPY --from=jbuilder /frontend/dist/ dist/
EXPOSE 8080
CMD ["./watch-together", "--tracing-level", "INFO"]
