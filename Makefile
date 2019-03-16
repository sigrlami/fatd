all: fatd fat-cli

dev: fatd-dev fat-cli-dev

REVISION     = $(shell ./revision)
FATD_LDFLAGS = "-X github.com/Factom-Asset-Tokens/fatd/flag.Revision=$(REVISION)"
CLI_LDFLAGS  = "-X main.Revision=$(REVISION)"

CLI_SRC  = go.mod go.sum $(wildcard cli/*.go)
FATD_SRC = go.mod go.sum $(filter-out $(CLI_SRC), $(wildcard *.go */*.go */*/*.go))

fatd: $(FATD_SRC)
	go build -ldflags=$(FATD_LDFLAGS) ./

fat-cli: $(CLI_SRC)
	go build -ldflags=$(CLI_LDFLAGS) -o fat-cli ./cli

fatd-dev: $(FATD_SRC)
	go build -race -ldflags=$(FATD_LDFLAGS) ./

fat-cli-dev: $(CLI_SRC)
	go build -race -ldflags=$(CLI_LDFLAGS) -o fat-cli ./cli

.PHONY: clean purge unpurge

clean:
	rm -f ./fatd ./fat-cli

DATE = $$(date -Ins)
purge: clean
	mv ./fatd.db /tmp/fatd.db.save-$(DATE)

unpurge:
	cp -aTn $$(ls /tmp/fatd.db.save-* -d | tail -n 1) ./fatd.db
