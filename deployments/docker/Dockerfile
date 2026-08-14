# Multi-stage build for the simulator binaries.
# Usage: docker build --build-arg APP=api -t simulator-api .
#        APP is one of: api | worker | cli | migrate
FROM golang:1.26 AS build
ARG APP=api
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sim ./cmd/${APP}

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 sim
USER sim
COPY --from=build /out/sim /usr/local/bin/sim
ENTRYPOINT ["sim"]
