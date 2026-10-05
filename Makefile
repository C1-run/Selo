.PHONY: build test vet fmt lint check install clean

build:
	go build -o selo ./cmd/selo/

test:
	SELO_HOME=$$(pwd) go test ./... -count=1

vet:
	go vet ./...

fmt:
	$$(go env GOROOT)/bin/gofmt -l -w $$(git ls-files '*.go')

lint: vet
	@$(MAKE) --no-print-directory fmt-check

fmt-check:
	@out=$$($$(go env GOROOT)/bin/gofmt -l $$(git ls-files '*.go')); \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

check: build vet fmt-check
	@$(MAKE) --no-print-directory test

install: build
	install -m 0755 selo /usr/local/bin/selo

clean:
	rm -f selo
