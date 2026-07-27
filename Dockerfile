# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS builder
ARG BINARY=api
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app ./cmd/${BINARY}

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/app /app/app

ENTRYPOINT ["/app/app"]
