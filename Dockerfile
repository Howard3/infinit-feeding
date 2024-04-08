ARG GO_VERSION=1
FROM golang:${GO_VERSION}-bookworm as builder

COPY ./scripts/setup_build_env.sh /tmp/setup_build_env.sh
RUN chmod +x /tmp/setup_build_env.sh && /tmp/setup_build_env.sh

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN npm i
RUN task build:dependencies 
RUN go build -v -o /run-app .


FROM debian:bookworm
RUN apt-get update && apt-get install -y ca-certificates && update-ca-certificates
RUN apt-get install -y zbar-tools
COPY --from=builder /run-app /usr/local/bin/
COPY --from=builder /usr/src/app/static ./static
CMD ["run-app"]
