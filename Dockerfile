FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -trimpath -o /flickmind .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /flickmind /flickmind
USER nonroot:nonroot
EXPOSE 7000
ENTRYPOINT ["/flickmind"]
