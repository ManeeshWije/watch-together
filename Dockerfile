# syntax=docker/dockerfile:1

# ----------------------------
# Rust backend builder
# ----------------------------
FROM --platform=$TARGETPLATFORM rust:slim AS rbuilder

WORKDIR /backend

RUN apt-get update && apt-get install -y \
    pkg-config \
    libssl-dev \
    && rm -rf /var/lib/apt/lists/*

COPY backend .

ENV SQLX_OFFLINE=true

RUN cargo build --release \
    && strip target/release/watch-together


# ----------------------------
# Frontend builder
# ----------------------------
FROM --platform=$TARGETPLATFORM node:20-slim AS jbuilder

WORKDIR /frontend

COPY frontend .

RUN npm install
RUN npm run build


# ----------------------------
# Runtime
# ----------------------------
FROM --platform=$TARGETPLATFORM debian:trixie-slim AS release

WORKDIR /app

RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    unzip \
    python3 \
    python3-pip \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Some code/tools may expect `python`
RUN ln -sf /usr/bin/python3 /usr/bin/python


# ----------------------------
# Install Deno
# Required by modern yt-dlp
# for YouTube JS challenges
# ----------------------------
ENV DENO_INSTALL=/usr/local

RUN curl -fsSL https://deno.land/install.sh | sh \
    && deno --version


# ----------------------------
# Install pinned yt-dlp
# ----------------------------
ARG YTDLP_VERSION=2026.07.04

RUN curl -fL \
    "https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/yt-dlp" \
    -o /usr/local/bin/yt-dlp \
    && chmod +x /usr/local/bin/yt-dlp \
    && yt-dlp --version


# ----------------------------
# Application
# ----------------------------
COPY --from=rbuilder \
    /backend/target/release/watch-together \
    /app/watch-together

COPY --from=jbuilder \
    /frontend/dist/ \
    /app/dist/


# Helpful build/runtime sanity checks
RUN ffmpeg -version | head -1 \
    && python --version \
    && deno --version \
    && yt-dlp --version


EXPOSE 8080

CMD ["./watch-together", "--tracing-level", "INFO"]
