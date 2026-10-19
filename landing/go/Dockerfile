FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/landing-api .

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/landing-api /landing-api
ENV LANDING_CRED_DIR=/credentials
VOLUME /credentials
EXPOSE 8080
ENTRYPOINT ["/landing-api"]
