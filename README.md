# Rmqbeat
[![Build Status](https://travis-ci.org/Jahor/rmqbeat.svg?branch=master)](https://travis-ci.org/Jahor/rmqbeat)
[![Go Report Card](https://goreportcard.com/badge/github.com/Jahor/rmqbeat)](https://goreportcard.com/report/github.com/Jahor/rmqbeat)
[![license](https://img.shields.io/github/license/Jahor/rmqbeat.svg)](https://github.com/Jahor/rmqbeat)

![Elastic Beats 6.3.1](https://img.shields.io/badge/Elastic%20Beats-v6.3.1-blue.svg)
![Golang 1.9](https://img.shields.io/badge/Golang-v1.9-blue.svg)

Welcome to Rmqbeat.

Rmqbeat allows to read messages over AMQP (from RabbitMQ or other server) and post them to Elasticsearch, Logstash, etc.

It preserves both payload and properties including headers.

It also supports `tracer` mode, which is designed to connect to `amq.rabbitmq.trace` node and receive trace messages activity.
In this mode it publishes message as it was sent (with original properties) and adds some additional information such as username, action performed (publish|deliver), etc.


It can also serve as a RabbitMQ log shipper when you subscribe to `amq.rabbitmq.log` exchange.
And in `event` mode it will read different rabbitmq events from `amq.rabbitmq.event`.

See [docs](docs/index.asciidoc) for more information on configuration parameters.

Ensure that this folder is at the following location:
`${GOPATH}/src/github.com/jahor/rmqbeat`

## Getting Started with Rmqbeat

### Requirements

* [Golang](https://golang.org/dl/) 1.9

### Init Project
To get running with Rmqbeat and also install the
dependencies, run the following command:

```
make setup
```

It will create a clean git history for each major step. Note that you can always rewrite the history if you wish before pushing your changes.

To push Rmqbeat in the git repository, run the following commands:

```
git remote set-url origin https://github.com/jahor/rmqbeat
git push origin master
```

For further development, check out the [beat developer guide](https://www.elastic.co/guide/en/beats/libbeat/current/new-beat.html).

### Build

To build the binary for Rmqbeat run the command below. This will generate a binary
in the same directory with the name rmqbeat.

```
make
```


### Run

To run Rmqbeat with debugging output enabled, run:

```
./rmqbeat -c rmqbeat.yml -e -d "*"
```


### Test

To test Rmqbeat, run the following command:

```
make testsuite
```

alternatively:
```
make unit-tests
make system-tests
make integration-tests
make coverage-report
```

The test coverage is reported in the folder `./build/coverage/`

### Update

Each beat has a template for the mapping in elasticsearch and a documentation for the fields
which is automatically generated based on `fields.yml` by running the following command.

```
make update
```


### Cleanup

To clean  Rmqbeat source code, run the following commands:

```
make fmt
make simplify
```

To clean up the build directory and generated artifacts, run:

```
make clean
```


### Clone

To clone Rmqbeat from the git repository, run the following commands:

```
mkdir -p ${GOPATH}/src/github.com/jahor/rmqbeat
git clone https://github.com/jahor/rmqbeat ${GOPATH}/src/github.com/jahor/rmqbeat
```


For further development, check out the [beat developer guide](https://www.elastic.co/guide/en/beats/libbeat/current/new-beat.html).


## Packaging

The beat frameworks provides tools to crosscompile and package your beat for different platforms. This requires [docker](https://www.docker.com/) and vendoring as described above. To build packages of your beat, run the following command:

```
make package
```

This will fetch and create all images required for the build process. The whole process to finish can take several minutes.
