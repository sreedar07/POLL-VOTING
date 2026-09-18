# Stage 1: build React frontend
FROM node:22-alpine AS frontend-build
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: build Go API
FROM golang:1.23-alpine AS backend-build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/pulsepoll ./cmd/server

# Stage 3: minimal production image
FROM alpine:3.21
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=backend-build /out/pulsepoll ./pulsepoll
COPY --from=frontend-build /src/frontend/dist ./frontend/dist
USER app
ENV PORT=10000
EXPOSE 10000
CMD ["./pulsepoll"]
