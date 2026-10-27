FROM golang:1.27.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /returns ./cmd/returns
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S returns && adduser -S -G returns returns && mkdir /data && chown returns:returns /data
COPY --from=build /returns /usr/local/bin/returns
USER returns
ENV DATA_DIR=/data
EXPOSE 8080
ENTRYPOINT ["returns"]
