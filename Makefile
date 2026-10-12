.PHONY: dev build docker clean

# 前端构建 → Go embed → 单二进制 ./sub2api-ticket
build:
	cd web && npm run build
	go build -trimpath -ldflags="-s -w" -o sub2api-ticket .

docker:
	docker compose build

dev:
	cd web && npm run dev

# 后端 + polling 调试
dev-go:
	TG_POLLING=true LISTEN_ADDR=:8080 go run .

clean:
	rm -f sub2api-ticket
	rm -rf internal/static/dist/* web/node_modules
