BEATNAME=rmqbeat
BEAT_DIR=github.com/jahor/rmqbeat
SYSTEM_TESTS=false
TEST_ENVIRONMENT=false
ES_BEATS?=./vendor/github.com/elastic/beats
GOPACKAGES=$(shell go list ${BEAT_DIR}/... | grep -v /vendor/)
#GOPACKAGES=$(shell glide novendor)
PREFIX?=.
NOTICE_FILE=NOTICE

# Path to the libbeat Makefile
-include $(ES_BEATS)/libbeat/scripts/Makefile

# Initial beat setup
.PHONY: setup
setup: copy-vendor
	make update

# Copy beats into vendor directory
.PHONY: copy-vendor
copy-vendor:
	mkdir -p vendor/github.com/elastic/
	cp -R ${GOPATH}/src/github.com/elastic/beats vendor/github.com/elastic/
	rm -rf vendor/github.com/elastic/beats/.git
	mkdir -p vendor/golang.org/x/
	cp -R ${GOPATH}/src/golang.org/x/text vendor/golang.org/x/
	rm -rf vendor/golang.org/x/text/.git
	mkdir -p vendor/github.com/streadway/
	cp -R ${GOPATH}/src/github.com/streadway/amqp vendor/github.com/streadway/
	rm -rf vendor/github.com/streadway/amqp/.git

.PHONY: git-init
git-init:
	git init
	git add README.md CONTRIBUTING.md
	git commit -m "Initial commit"
	git add LICENSE
	git commit -m "Add the LICENSE"
	git add .gitignore
	git commit -m "Add git settings"
	git add .
	git reset -- .travis.yml
	git commit -m "Add rmqbeat"
	git add .travis.yml
	git commit -m "Add Travis CI"

# This is called by the beats packer before building starts
.PHONY: before-build
before-build:

# Collects all dependencies and then calls update
.PHONY: collect
collect:
