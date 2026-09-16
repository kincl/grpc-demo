FROM golang:1.26 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY gen/ gen/
COPY backend/ backend/
RUN CGO_ENABLED=0 GOOS=linux go build -o /doughnut-backend ./backend

FROM gcr.io/distroless/static-debian12
COPY --from=builder /doughnut-backend /doughnut-backend
EXPOSE 50051
ENTRYPOINT ["/doughnut-backend"]
