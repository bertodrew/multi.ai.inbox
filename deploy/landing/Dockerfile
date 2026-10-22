# Public site served by Caddy, which also terminates TLS for the API.
# Build from the repository root: docker build -f deploy/landing/Dockerfile .
#   /         the Next.js static export from web/ (home, docs, pricing, legal)
#   /legacy/  the previous landing imported from the old server, kept for reference
FROM node:22.22.0-alpine AS site
WORKDIR /src
COPY openapi/ openapi/
COPY web/package.json web/package-lock.json web/.npmrc web/
RUN cd web && npm ci --no-audit --no-fund
COPY web/ web/
RUN cd web && npm run build

FROM caddy:2.11.7-alpine
COPY --from=site /src/web/out/ /srv/
COPY landing/ /srv/legacy/
