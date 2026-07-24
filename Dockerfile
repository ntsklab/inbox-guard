# Stage 1: Build
FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /inbox-guard .

# Stage 2: Minimal runtime
FROM scratch
COPY --from=builder /inbox-guard /inbox-guard
EXPOSE 3000
ENTRYPOINT ["/inbox-guard"]
