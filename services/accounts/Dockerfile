FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY shared/ ./shared/
COPY services/accounts/ ./services/accounts/
WORKDIR /app/services/accounts
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/accounts ./cmd/server

FROM gcr.io/distroless/static-debian12
COPY --from=builder /bin/accounts /accounts
EXPOSE 8082
ENTRYPOINT ["/accounts"]
