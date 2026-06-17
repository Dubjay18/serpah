FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ ./shared/
COPY services/gateway/ ./services/gateway/
WORKDIR /app/services/gateway
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/gateway ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/gateway /gateway
EXPOSE 8080
ENTRYPOINT ["/gateway"]
