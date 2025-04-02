#https://docs.docker.com/build/building/multi-stage/

ARG PULL_REGISTRY=docker.io
FROM $PULL_REGISTRY/golang:1.24.1-alpine AS builder

WORKDIR /nexus-initlzr

ENV CGO_ENABLED=0
ENV GOCACHE=/root/.cache/go-build
ENV GOMODCACHE=/root/.cache/go-build
ADD go.* ./
RUN  --mount=type=cache,target=/root/.cache/go-build  go mod download -x
COPY . ./
RUN  --mount=type=cache,target=/root/.cache/go-build  go mod tidy
WORKDIR /nexus-initlzr/main
RUN  --mount=type=cache,target=/root/.cache/go-build \
GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /out/nexus-initlzr .


#Second build layer
FROM $PULL_REGISTRY/alpine:3.21.3
COPY --from=builder /out/nexus-initlzr /nexus-initlzr
COPY --from=builder /nexus-initlzr/main/config.json /config.json
RUN mkdir /cfg
#ARG CACHE_TS=default_ts
#RUN ls -la /
ENTRYPOINT [ "/nexus-initlzr" ]
