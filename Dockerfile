FROM lukemathwalker/cargo-chef:latest as chef

ARG AWS_URL
ARG AWS_ACCESS_KEY_ID
ARG AWS_REGION
ARG AWS_SECRET_ACCESS_KEY
ARG AWS_S3_BUCKET
ARG PASSWORD

ENV AWS_URL=$AWS_URL
ENV AWS_ACCESS_KEY_ID=$AWS_ACCESS_KEY_ID
ENV AWS_REGION=$AWS_REGION
ENV AWS_SECRET_ACCESS_KEY=$AWS_SECRET_ACCESS_KEY
ENV AWS_S3_BUCKET=$AWS_S3_BUCKET
ENV PASSWORD=$PASSWORD

WORKDIR /app

FROM chef AS planner
COPY ./Cargo.toml ./Cargo.lock ./
COPY ./src ./src
RUN cargo chef prepare

FROM chef AS builder
COPY --from=planner /app/recipe.json .
RUN cargo chef cook --release
COPY . .
RUN cargo build --release
RUN mv ./target/release/server ./app

FROM debian:latest AS runtime
WORKDIR /app
# Install necessary dependencies
RUN apt-get update && apt-get install -y libssl-dev ca-certificates
COPY --from=builder /app/app /usr/local/bin/
ENTRYPOINT ["/usr/local/bin/app"]
