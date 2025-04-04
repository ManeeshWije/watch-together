# Use buildx-compatible base images
FROM --platform=$TARGETPLATFORM rust:slim AS rbuilder
WORKDIR /backend
COPY backend .
ENV SQLX_OFFLINE=true
# Install build dependencies
RUN apt-get update && apt-get install -y pkg-config libssl-dev
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
RUN add-apt-repository ppa:tomtomtom/yt-dlp && apt-get update && apt install yt-dlp
COPY --from=rbuilder /backend/target/release/watch-together .
COPY --from=jbuilder /frontend/dist/ dist/
EXPOSE 8080
CMD ["./watch-together", "--tracing-level", "INFO"]
