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

COPY services/identities/go.mod services/identities/go.sum ./services/identities/


RUN --mount=type=secret,id=sshkey,target=/root/.ssh/id,mode=0600,required cd services/identities && go mod download

COPY . .

RUN make build-identities

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/build/cex-identities /app/cex-identities

RUN useradd -m appuser
USER appuser

ENTRYPOINT ["/app/cex-identities"]
