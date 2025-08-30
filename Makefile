## MIT License - Copyright (c) 2025 TheSkyscape

BINARY := workspace
WORKER_BINARY := worker
WORKER_EMBED_PATH := internal/worker/resources/worker

.PHONY: all clean build-worker copy-worker

all: build/$(WORKER_BINARY) copy-worker build/$(BINARY)

clean:
	rm -rf build
	rm -f $(WORKER_EMBED_PATH)

# Build workspace binary (depends on embedded worker)
build/$(BINARY): copy-worker
	@mkdir -p build
	@echo "Building workspace with embedded worker..."
	go build -o $@ .
	@echo "Workspace built successfully with embedded worker"

# Build worker binary first
build/$(WORKER_BINARY):
	@mkdir -p build
	@echo "Building worker service..."
	@cd ../worker && go build -o ../workspace/build/$(WORKER_BINARY) .
	@echo "Worker binary built successfully"

# Copy worker binary for embedding
copy-worker: build/$(WORKER_BINARY)
	@echo "Copying worker binary for embedding..."
	@mkdir -p internal/worker/resources
	@cp build/$(WORKER_BINARY) $(WORKER_EMBED_PATH)
	@echo "Worker binary copied to $(WORKER_EMBED_PATH)"