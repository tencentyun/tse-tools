TARGET=all_sync t_sync
MAIN_MODULE=github.com/tencentyun/zk2zk
GO=CGO_ENABLED=0 go

all: $(TARGET)

.PHONY: all_sync
all_sync:
	$(GO) build -o $@/$@ $(MAIN_MODULE)/$@

.PHONY: t_sync
t_sync:
	$(GO) build -o $@/$@ $(MAIN_MODULE)/$@