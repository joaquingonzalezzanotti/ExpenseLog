FROM public.ecr.aws/docker/library/golang:1.23.2-alpine3.20 AS builder

WORKDIR /app

# Cache dependencies in a dedicated layer.
COPY go.mod go.sum ./
RUN go mod download

# Copy source only after dependencies are cached.
COPY . .

# Build a small Linux binary.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/expenselog ./cmd/expenselog

FROM public.ecr.aws/docker/library/alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates && mkdir -p /app/data

COPY --from=builder /app/expenselog /app/expenselog

EXPOSE 8080

CMD ["./expenselog"]
