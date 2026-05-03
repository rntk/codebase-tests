# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25
ARG NODE_VERSION=24
ARG GOPLS_VERSION=v0.21.1

FROM node:${NODE_VERSION}-bookworm-slim AS web-build
WORKDIR /src/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM golang:${GO_VERSION}-bookworm AS go-build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-build /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/review-server ./cmd/review-server

FROM golang:${GO_VERSION}-bookworm AS gopls-build
ARG GOPLS_VERSION
RUN GOBIN=/out go install golang.org/x/tools/gopls@${GOPLS_VERSION}

FROM golang:${GO_VERSION}-bookworm AS runtime
ARG NODE_VERSION
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl git gnupg python3 python3-pytest python3-coverage \
	&& mkdir -p /etc/apt/keyrings \
	&& curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
	&& echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_VERSION}.x nodistro main" > /etc/apt/sources.list.d/nodesource.list \
	&& apt-get update \
	&& apt-get install -y --no-install-recommends nodejs \
	&& npm install -g typescript typescript-language-server \
	&& apt-get purge -y curl gnupg && apt-get autoremove -y \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=go-build /out/review-server /usr/local/bin/review-server
COPY --from=gopls-build /out/gopls /usr/local/bin/gopls
COPY --from=web-build /src/web/dist /opt/review-server/web/dist

WORKDIR /workspace

ENV REVIEW_SERVER_ADDR=:8080
ENV REVIEW_SERVER_STATIC_DIR=/opt/review-server/web/dist

EXPOSE 8080

ENTRYPOINT ["review-server"]
CMD ["-project", "/workspace", "-plugins", "go"]
