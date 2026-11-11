.PHONY: test install server docker
test:
	go vet ./...
	go test ./...
install:
	go install ./cmd/tasktui
server:
	CGO_ENABLED=0 go build -o tasktui-server ./cmd/tasktui-server
docker:
	docker build -t tasktui .
