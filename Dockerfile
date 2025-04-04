FROM --platform=$BUILDPLATFORM rust:latest AS rbuilder
WORKDIR /backend
COPY backend .

RUN cargo install sqlx-cli
ENV SQLX_OFFLINE true

# Set the target architecture based on the platform
ARG TARGETARCH
RUN rustup target add aarch64-unknown-linux-gnu x86_64-unknown-linux-gnu

# Build for the target platform
RUN if [ "$TARGETARCH" = "arm64" ]; then \
        cargo build --release --target=aarch64-unknown-linux-gnu; \
    else \
        cargo build --release --target=x86_64-unknown-linux-gnu; \
    fi

# Strip binary to reduce size
RUN if [ "$TARGETARCH" = "arm64" ]; then \
        strip target/aarch64-unknown-linux-gnu/release/watch-together; \
    else \
        strip target/x86_64-unknown-linux-gnu/release/watch-together; \
    fi

# Node.js frontend build stage
FROM --platform=$BUILDPLATFORM node:20-slim AS jbuilder
WORKDIR /frontend
COPY frontend .
RUN npm install
RUN npm run build

# Final stage using distroless
FROM --platform=$TARGETPLATFORM gcr.io/distroless/cc-debian12:latest AS release
WORKDIR /app

# Copy the correct binary based on architecture
ARG TARGETARCH
COPY --from=rbuilder /backend/target/aarch64-unknown-linux-gnu/release/watch-together ./watch-together
COPY --from=jbuilder /frontend/dist/ dist/

EXPOSE 8080
CMD ["./watch-together", "--tracing-level", "INFO", "--run-migrations"]
