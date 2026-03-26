.PHONY: web-build go-build build clean

web-build:
	cd web && npm run build

go-build: web-build
	go build -o bin/lingtai .

build: go-build

cross-compile: web-build
	GOOS=darwin GOARCH=arm64 go build -o bin/lingtai-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build -o bin/lingtai-darwin-x64 .
	GOOS=linux GOARCH=amd64 go build -o bin/lingtai-linux-x64 .
	GOOS=linux GOARCH=arm64 go build -o bin/lingtai-linux-arm64 .
	GOOS=windows GOARCH=amd64 go build -o bin/lingtai-win32-x64.exe .

clean:
	rm -rf bin/ web/dist/
