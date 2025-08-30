## MIT License - Copyright (c) 2025 TheSkyscape

BINARY := workspace

.PHONY: all clean build

all: build/$(BINARY)

clean:
	rm -rf build

# Build workspace binary
build/$(BINARY):
	@mkdir -p build
	@echo "Building workspace application..."
	go build -o $@ .
	@echo "Workspace built successfully"