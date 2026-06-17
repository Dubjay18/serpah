FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ ./shared/
COPY services/payments/ ./services/payments/
WORKDIR /app/services/payments
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/payments ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/payments /payments
EXPOSE 8084
ENTRYPOINT ["/payments"]
