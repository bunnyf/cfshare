.PHONY: build install clean test

BINARY_NAME=cfshare
INSTALL_PATH=$(HOME)/bin

build:
	go build -o $(BINARY_NAME) .

install: build
	mkdir -p $(INSTALL_PATH)
	cp $(BINARY_NAME) $(INSTALL_PATH)/
	@echo "✅ Installed to $(INSTALL_PATH)/$(BINARY_NAME)"
	@echo ""
	@echo "确保 ~/bin 在 PATH 中:"
	@echo "  echo 'export PATH=\"\$$HOME/bin:\$$PATH\"' >> ~/.zshrc"
	@echo "  source ~/.zshrc"

uninstall:
	rm -f $(INSTALL_PATH)/$(BINARY_NAME)
	@echo "✅ Uninstalled"

clean:
	rm -f $(BINARY_NAME)

clean-all: clean
	rm -rf ~/.cfshare

test:
	go test ./...

# 开发用：直接运行
run:
	go run . $(ARGS)
