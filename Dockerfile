FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -mod=mod -trimpath -ldflags="-s -w" -o /server ./cmd/server
FROM alpine:3.22
RUN adduser -D -u 10001 libraryhub
# Reuse certificates and Go timezone data without accessing Alpine repositories.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/local/go/lib/time/zoneinfo.zip /usr/local/share/zoneinfo.zip
WORKDIR /app
COPY --from=build /server ./server
COPY web ./web
USER libraryhub
EXPOSE 8080
ENV TZ=Europe/Moscow
ENV ZONEINFO=/usr/local/share/zoneinfo.zip
ENTRYPOINT ["./server"]
