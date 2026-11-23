# Build the web app.
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# Build the server, with the web app embedded.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gemba ./cmd/gemba

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /gemba /gemba
# Deployed containers refuse requests with no signed-in identity.
# For a local demo, run with -e GEMBA_REQUIRE_AUTH=0.
ENV GEMBA_ADDR=0.0.0.0:8080 GEMBA_REQUIRE_AUTH=1
EXPOSE 8080
ENTRYPOINT ["/gemba"]
