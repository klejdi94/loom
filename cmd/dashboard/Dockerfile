FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /dashboard ./cmd/dashboard

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=build /dashboard /dashboard
EXPOSE 8081
ENTRYPOINT ["/dashboard"]
