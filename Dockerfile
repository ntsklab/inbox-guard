# Stage 1: Build
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=${VERSION}" -o /inbox-guard .

# Stage 2: Minimal runtime
FROM scratch
COPY --from=builder /inbox-guard /inbox-guard
EXPOSE 3000
ENTRYPOINT ["/inbox-guard"]
