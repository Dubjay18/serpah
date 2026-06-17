FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ ./shared/
COPY services/ledger/ ./services/ledger/
WORKDIR /app/services/ledger
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/ledger ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/ledger /ledger
EXPOSE 8083
ENTRYPOINT ["/ledger"]
