FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server && CGO_ENABLED=0 go build -o /out/vaultly ./cmd/vaultly

FROM alpine:3.20
RUN adduser -D -u 10001 app
COPY --from=build /out/server /out/vaultly /usr/local/bin/
USER app
EXPOSE 8080
CMD ["server"]
