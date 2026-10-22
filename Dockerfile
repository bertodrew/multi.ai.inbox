# One image with the Go platform binary and the Rust gatekeeper.
FROM rust:1.90-bookworm AS gatekeeper
WORKDIR /src
COPY gatekeeper/ .
RUN cargo build --release --locked

FROM golang:1.25-bookworm AS mai
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/mai ./cmd/mai

FROM gcr.io/distroless/cc-debian12:nonroot
COPY --from=mai /out/mai /usr/local/bin/mai
COPY --from=gatekeeper /src/target/release/gatekeeper /usr/local/bin/gatekeeper
ENV MAI_GATEKEEPER_BIN=/usr/local/bin/gatekeeper \
    MAI_BLOB_DIR=/var/lib/mai/blobs \
    MAI_HTTP_ADDR=:8080 \
    MAI_SMTP_ADDR=:2525
EXPOSE 8080 2525
ENTRYPOINT ["/usr/local/bin/mai"]
CMD ["serve"]
