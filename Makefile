BINARY := massage-gate

.PHONY: all web build clean

all: build

web:
	cd web-src && npm install --no-audit --no-fund && npm run build
	rm -rf internal/web/dist && cp -r web-src/dist internal/web/dist

build: web
	go build -o $(BINARY) ./cmd/server

# 仅编译 Go(使用仓库内已有前端产物)
go:
	go build -o $(BINARY) ./cmd/server

clean:
	rm -f $(BINARY) massage-gate.exe
	rm -rf web-src/dist web-src/node_modules
