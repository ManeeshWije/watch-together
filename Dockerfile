FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build

WORKDIR /app

COPY . /app

RUN go mod download

RUN go get -u

RUN go mod tidy

# Use build arguments to set target platform
ARG TARGETOS
ARG TARGETARCH
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o watch-together

FROM --platform=$TARGETPLATFORM alpine:3.18

# Install any necessary packages, like certificates
RUN apk add --no-cache \
    ca-certificates \
    ffmpeg

WORKDIR /app

COPY --from=build /app /app

EXPOSE 8080

ENV AWS_URL=""
ENV AWS_ACCESS_KEY_ID=""
ENV AWS_REGION=""
ENV AWS_SECRET_ACCESS_KEY=""
ENV AWS_S3_BUCKET=""
ENV DATABASE_PUBLIC_URL=""
ENV CLIENT_ID=""
ENV CLIENT_SECRET=""
ENV GOOGLE_REDIRECT_URL=""

CMD ["/app/watch-together"]
