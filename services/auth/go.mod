module github.com/Dubjay18/seraph/services/auth

go 1.25.0

require github.com/jackc/pgx/v5 v5.6.0

require (
	github.com/Dubjay18/seraph/shared v0.0.0
	github.com/go-chi/chi/v5 v5.3.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/swaggo/http-swagger/v2 v2.0.2
	github.com/swaggo/swag v1.16.6
	go.uber.org/zap v1.28.0
	golang.org/x/crypto v0.53.0
)

require (
	cloud.google.com/go/auth v0.18.2 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.14 // indirect
	github.com/rabbitmq/amqp091-go v1.10.0 // indirect
)

replace github.com/Dubjay18/seraph/shared => ../../shared

require (
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/KyleBanks/depth v1.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.20.0 // indirect
	github.com/go-openapi/spec v0.20.6 // indirect
	github.com/go-openapi/swag v0.19.15 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/gax-go/v2 v2.22.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.6 // indirect
	github.com/rogpeppe/go-internal v1.15.0 // indirect
	github.com/swaggo/files/v2 v2.0.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.67.0 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	go.uber.org/multierr v1.10.0 // indirect
	golang.org/x/mod v0.36.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/oauth2 v0.36.0
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	golang.org/x/tools v0.45.0 // indirect
	google.golang.org/api v0.272.0
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260610212136-7ab31c22f7ad // indirect
	google.golang.org/grpc v1.81.1 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)
