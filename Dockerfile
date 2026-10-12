# ---- 前端构建 ----
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci || npm install
COPY web/ ./
RUN npm run build

# ---- 后端构建 ----
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 覆盖 embed 目录为前端构建产物
COPY --from=web /src/internal/static/dist ./internal/static/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sub2api-ticket .

# ---- 运行镜像 ----
FROM alpine:3.20
RUN adduser -D -u 10001 app && apk add --no-cache ca-certificates tzdata
COPY --from=build /out/sub2api-ticket /usr/local/bin/sub2api-ticket
USER app
WORKDIR /data
ENV DB_PATH=/data/ticket.db LISTEN_ADDR=:8080
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["sub2api-ticket"]
