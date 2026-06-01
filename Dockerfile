# Multi-stage build for Linux containers
FROM golang:1.20-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/axiom-server

FROM alpine:3.18
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/axiom-server ./axiom-server
# copy static assets
COPY frontend ./frontend
COPY loot-images ./loot-images
EXPOSE 8080
ENV PORT=8080
CMD ["./axiom-server"]
