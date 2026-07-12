.PHONY: init build test demo clean

init:
	cd go-core && go mod tidy
	cd py-orchestrator && pip install -e . && pip install -r requirements.txt

build:
	cd go-core && go build -o ../bin/blastradius ./cmd/blastradius

test:
	cd go-core && go test ./... -v
	cd py-orchestrator && pytest -v

demo: build
	cd py-orchestrator && python -m orchestrator.cli scan \
		--repo ../examples/tinyshop \
		--diff WORKING_TREE \
		--go-binary ../bin/blastradius \
		--verbose

clean:
	rm -rf bin/*
	find . -name "*.db" -delete