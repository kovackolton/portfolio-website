FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test ./... && go vet ./... && CGO_ENABLED=0 go build -mod=readonly -o /api .
FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D app
USER app
COPY --from=build /api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
