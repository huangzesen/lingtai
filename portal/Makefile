.PHONY: web-deps web-build go-build build clean

web-deps:
	cd web && npm install

web-build: web-deps
	cd web && npm run build

go-build: web-build
	go build -o bin/lingtai-portal .

build: go-build

cross-compile: web-build
	GOOS=darwin GOARCH=arm64 go build -o bin/lingtai-portal-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build -o bin/lingtai-portal-darwin-x64 .
	GOOS=linux GOARCH=amd64 go build -o bin/lingtai-portal-linux-x64 .
	GOOS=linux GOARCH=arm64 go build -o bin/lingtai-portal-linux-arm64 .
	GOOS=windows GOARCH=amd64 go build -o bin/lingtai-portal-win32-x64.exe .

clean:
	rm -rf bin/ web/dist/
