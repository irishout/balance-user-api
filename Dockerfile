FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/app ./cmd/app

FROM alpine:3.22
COPY --from=build /bin/app /bin/app
EXPOSE 8080
CMD ["/bin/app"]