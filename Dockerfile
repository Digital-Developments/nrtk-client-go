FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

LABEL app.nrtk-client-go.vendor="Digital Developments"
LABEL app.nrtk-client-go.version="0.1"
LABEL app.nrtk-client-go.release-date="2025-04-13"

WORKDIR /app
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0

COPY go.mod ./
COPY go.sum ./
COPY main.go ./

RUN go mod download

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /app/nrtk-client

EXPOSE $HTTP_SERVER_PORT

CMD [ "/app/nrtk-client" ]