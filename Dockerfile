# syntax=docker/dockerfile:1

FROM golang:1.22 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/labmonitor ./cmd/server

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/labmonitor /labmonitor
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/labmonitor", "-listen", ":8080"]
