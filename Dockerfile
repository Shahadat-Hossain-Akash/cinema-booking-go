# ---- build stage ----
FROM golang:1.22-alpine AS builder
WORKDIR /app

# go.sum is gitignored in this repo, so it may not exist in the build context.
# Copy full source first, then tidy — regenerates go.sum from go.mod, works
# whether go.sum is present or not.
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux go build -o server .

# ---- run stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /app/server .

# Railway injects PORT at runtime; app reads it via os.Getenv("PORT").
EXPOSE 8080
CMD ["./server"]
