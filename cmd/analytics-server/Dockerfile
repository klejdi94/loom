FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /analytics-server ./cmd/analytics-server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
COPY --from=build /analytics-server /analytics-server
EXPOSE 8080
ENTRYPOINT ["/analytics-server"]
