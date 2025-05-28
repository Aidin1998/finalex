FROM golang:1.24-bookworm AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y make openssh-client git


ARG GOPRIVATE=github.com/litebittech/*
ENV GOPRIVATE=${GOPRIVATE}

RUN mkdir -p /root/.ssh && \
    chmod 700 /root/.ssh && \
    echo "Host github.com\n\tStrictHostKeyChecking no\n\tIdentityFile /root/.ssh/id\n" > /root/.ssh/config && \
    chmod 600 /root/.ssh/config

RUN git config --global url."git@github.com:".insteadOf "https://github.com/"

COPY services/bookkeeper/go.mod services/bookkeeper/go.sum ./services/bookkeeper/


RUN --mount=type=secret,id=sshkey,target=/root/.ssh/id,mode=0600,required cd services/bookkeeper && go mod download

COPY . .

RUN make build-bookkeeper

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/build/cex-bookkeeper /app/cex-bookkeeper

RUN useradd -m appuser
USER appuser

ENTRYPOINT ["/app/cex-bookkeeper"]
