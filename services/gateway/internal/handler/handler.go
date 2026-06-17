package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"go.uber.org/zap"
)

// ServiceSpec holds a service name and its base URL for OpenAPI aggregation.
type ServiceSpec struct {
	Name string
	URL  string
}

type Handler struct {
	authProxy    *httputil.ReverseProxy
	log          *zap.Logger
	upstreamSpecs []ServiceSpec
}

func New(authServiceURL *url.URL, log *zap.Logger, upstreams []ServiceSpec) *Handler {
	proxy := httputil.NewSingleHostReverseProxy(authServiceURL)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error("reverse proxy error", zap.String("path", r.URL.Path), zap.Error(err))
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"bad gateway"}`))
	}

	return &Handler{
		authProxy:    proxy,
		log:          log,
		upstreamSpecs: upstreams,
	}
}

// Health godoc
//
//	@Summary		Gateway health check
//	@Description	Returns the health status of the API gateway
//	@Tags			gateway
//	@Produce		json
//	@Success		200	{object}	map[string]string
//	@Router			/gateway/health [get]
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok","service":"gateway"}`))
}

func (h *Handler) ProxyAuth(w http.ResponseWriter, r *http.Request) {
	h.log.Info("proxying auth request", zap.String("method", r.Method), zap.String("path", r.URL.Path))
	h.authProxy.ServeHTTP(w, r)
}

// AggregatedSpec fetches OpenAPI specs from all upstream services and merges them.
//
//	@Summary		Aggregated OpenAPI spec
//	@Description	Returns a merged OpenAPI 3.0 spec from all services
//	@Tags			docs
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/docs/openapi.json [get]
func (h *Handler) AggregatedSpec(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 5 * time.Second}

	merged := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "Seraph API",
			"description": "Aggregated API documentation for all Seraph microservices",
			"version":     "1.0.0",
		},
		"paths":      map[string]interface{}{},
		"components": map[string]interface{}{"schemas": map[string]interface{}{}},
		"tags":       []interface{}{},
	}

	paths := merged["paths"].(map[string]interface{})
	schemas := merged["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	tags := merged["tags"].([]interface{})

	for _, svc := range h.upstreamSpecs {
		specURL := fmt.Sprintf("%s/openapi.json", svc.URL)
		resp, err := client.Get(specURL)
		if err != nil {
			h.log.Warn("failed to fetch spec from service", zap.String("service", svc.Name), zap.String("url", specURL), zap.Error(err))
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			h.log.Warn("failed to read spec body", zap.String("service", svc.Name), zap.Error(err))
			continue
		}

		var spec map[string]interface{}
		if err := json.Unmarshal(body, &spec); err != nil {
			h.log.Warn("failed to parse spec JSON", zap.String("service", svc.Name), zap.Error(err))
			continue
		}

		// Merge paths
		if svcPaths, ok := spec["paths"].(map[string]interface{}); ok {
			for path, def := range svcPaths {
				paths[path] = def
			}
		}

		// Merge schemas
		if comps, ok := spec["components"].(map[string]interface{}); ok {
			if svcSchemas, ok := comps["schemas"].(map[string]interface{}); ok {
				for name, schema := range svcSchemas {
					schemas[name] = schema
				}
			}
		}

		// Collect tags
		if svcTags, ok := spec["tags"].([]interface{}); ok {
			tags = append(tags, svcTags...)
		}
	}

	merged["tags"] = tags

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(merged)
}
