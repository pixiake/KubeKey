GO = go
GO_GET = $(GO) get -v
GO_TEST = $(GO) test -v


all: test

dep:
	-$(GO_GET) github.com/smartystreets/goconvey/convey

test: dep
	$(GO_TEST)

.PHONY: all dep test

