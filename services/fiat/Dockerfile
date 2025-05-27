# Dockerfile for fiat service
FROM alpine:latest
# ...add your build instructions here...

# Jaeger tracing
ENV JAEGER_AGENT_HOST=jaeger
ENV JAEGER_AGENT_PORT=6831

# Prometheus metrics exposed at /metrics
# Logs: JSON to stdout

# Example entrypoint
CMD ["/app/cmd/fiat-gateway/main"]
