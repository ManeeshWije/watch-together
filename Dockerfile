FROM golang:1.23-alpine AS build

WORKDIR /app

COPY . /app

RUN go mod download

RUN go build

FROM alpine:3.18

# Install any necessary packages, like certificates
RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=build /app /app

EXPOSE 8080

ENV AWS_URL=""
ENV AWS_ACCESS_KEY_ID=""
ENV AWS_REGION=""
ENV AWS_SECRET_ACCESS_KEY=""
ENV AWS_S3_BUCKET=""
ENV PASSWORD=""
ENV COOKIE_VAL=""

CMD ["/app/watch-together"]
