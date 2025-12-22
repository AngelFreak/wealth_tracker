# ==========================================
# Stage 1: Build Go binary
# ==========================================
FROM golang:1.24-alpine AS go-builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o server \
    ./cmd/server

# ==========================================
# Stage 2: Build Tailwind CSS
# ==========================================
FROM node:20-alpine AS css-builder

WORKDIR /build

# Copy package files and install dependencies (including devDependencies for tailwindcss)
COPY package.json ./
RUN npm install

# Copy Tailwind config and source files
COPY tailwind.config.js ./
COPY web/static/css/input.css web/static/css/
COPY web/templates/ web/templates/
COPY web/static/js/ web/static/js/

# Build minified CSS
RUN npx tailwindcss -i web/static/css/input.css -o web/static/css/app.css --minify

# ==========================================
# Stage 3: Runtime image
# ==========================================
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS and tzdata for timezones
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=go-builder /build/server .

# Copy templates
COPY web/templates/ web/templates/

# Copy static files
COPY web/static/ web/static/

# Copy built CSS from css-builder (overwrite source CSS)
COPY --from=css-builder /build/web/static/css/app.css web/static/css/app.css

# Create data directory
RUN mkdir -p data

# Environment defaults
ENV HOST=0.0.0.0
ENV PORT=8080
ENV DB_PATH=/app/data/wealth.db
ENV ENV=production

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q --spider http://localhost:8080/health || exit 1

# Run the server
CMD ["./server"]
