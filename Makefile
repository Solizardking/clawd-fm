VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINARY   ?= clawdamp
LDFLAGS  := -s -w -X main.version=$(VERSION)
IMAGE    ?= ghcr.io/8bitsats/clawd-fm:$(VERSION)

.PHONY: build test vet lint fmt check clean install deps run run-daemon init airdrop docker docker-run deploy deploy-systemd

deps:
	go mod download && go mod tidy

build: deps
	go build -trimpath -ldflags="$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	@if command -v staticcheck >/dev/null 2>&1; then staticcheck ./...; else echo "staticcheck not installed"; fi

fmt:
	gofmt -l -w .

check: fmt vet test

clean:
	rm -f $(BINARY)

install: build
	install -d $(HOME)/.local/bin
	install -m 755 $(BINARY) $(HOME)/.local/bin/$(BINARY)

run: build
	./$(BINARY)

run-daemon: build
	CLAWD_FM_NAME="CLAWD FM" CLAWD_FM_ADDR=":9669" CLAWD_SOL_NETWORK="devnet" ./$(BINARY) daemon

init: build
	./$(BINARY) init

airdrop: build
	./$(BINARY) airdrop

docker:
	docker build -t $(IMAGE) -f deploy/Dockerfile .

docker-run: docker
	docker run --rm -it -p 9669:9669 -e CLAWD_SOL_NETWORK=devnet -v $(PWD)/.clawd-fm:/data $(IMAGE) daemon

deploy:
	docker-compose -f deploy/docker-compose.yml up -d

deploy-systemd: build install
	sudo cp deploy/clawdfm.service /etc/systemd/system/
	sudo systemctl daemon-reload && sudo systemctl enable --now clawdfm
