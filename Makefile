## MIT License - Copyright (c) 2025 TheSkyscape

BINARY := workspace

.PHONY: all clean

all: build/$(BINARY)

clean:
	rm -rf build

build/$(BINARY):
	@mkdir -p build
	go build -o $@ .