.PHONY: test install server docker demo capturas site
test:
	go vet ./...
	go test ./...
install:
	go install ./cmd/tasktui
server:
	CGO_ENABLED=0 go build -o tasktui-server ./cmd/tasktui-server
docker:
	docker build -t tasktui .
demo:
	go run ./cmd/tasktui demo
capturas:
	sh scripts/capturas.sh
site:
	cd site && python3 -m http.server 8931 --bind 127.0.0.1
