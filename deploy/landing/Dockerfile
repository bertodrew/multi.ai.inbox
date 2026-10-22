# Static landing served by Caddy, which also terminates TLS for the API.
# Build from the repository root: docker build -f deploy/landing/Dockerfile .
FROM caddy:2.11.7-alpine
COPY landing/ /srv/
