FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ ./shared/
COPY services/auth/ ./services/auth/
WORKDIR /app/services/auth
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/auth ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/auth /auth
EXPOSE 8081
ENTRYPOINT ["/auth"]
