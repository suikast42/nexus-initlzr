#https://docs.docker.com/build/building/multi-stage/
FROM golang:1.19-alpine AS builder

WORKDIR /nexus-initlzr

COPY . .

RUN go mod tidy
WORKDIR /nexus-initlzr/main
RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o nexus-initlzr

#docker build --target builder
FROM alpine:3.16.2
COPY --from=builder /nexus-initlzr/main/nexus-initlzr /nexus-initlzr
COPY --from=builder /nexus-initlzr/main/config.json /config.json
RUN mkdir /cfg
ENTRYPOINT [ "/nexus-initlzr" ]