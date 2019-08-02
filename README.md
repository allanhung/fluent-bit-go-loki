# fluent-bit loki output plugin

[![Build Status](https://travis-ci.org/cosmo0920/fluent-bit-go-loki.svg?branch=master)](https://travis-ci.org/cosmo0920/fluent-bit-go-loki)
[![Build status](https://ci.appveyor.com/api/projects/status/6s9itaxvrkos11sx/branch/master?svg=true)](https://ci.appveyor.com/project/cosmo0920/fluent-bit-go-loki/branch/master)

Windows binaries are available in [release pages](https://github.com/cosmo0920/fluent-bit-go-loki/releases).

This plugin works with fluent-bit's go plugin interface. You can use fluent-bit loki to ship logs into grafana datasource with loki.

The configuration typically looks like:

```graphviz
fluent-bit --> loki --> grafana <-- other grafana sources
```

# Usage

```bash
$ fluent-bit -e /path/to/built/out_loki.so -c fluent-bit.conf
```

Or,

Edit Url parameter in the [OUTPUT] section in the [configuration](docker/fluent-bit-loki.conf).

```bash
$ docker build . -t fluent-bit/loki-plugin
```

and then,

```bash
$ docker run -it -e="LOKI_URL=http://[YOURHOST]:[YOURPORT]/api/prom/push" fluent-bit/loki-plugin
```

# Prerequisites

* Go 1.11+
* gcc (for cgo)

## Building

```bash
$ make
```

### Configuration Options

| Key           | Description                                   | Default                             |
| --------------|-----------------------------------------------|-------------------------------------|
| Url           | Url of loki server API endpoint               | http://localhost:3100/api/prom/push |
| BatchWait     | Waiting time for batch operation (unit: msec) | 10 milliseconds                     |
| Url           | Batch size for batch operation (unit: KiB)    | 10 KiB                              |
| Labels        | labels for API requests                       | job="fluent-bit" (describe below)   |

Example:

add this section to fluent-bit.conf

```properties
[Output]
    Name loki
    Match *
    Url http://localhost:3100/api/prom/push
    BatchWait 10 # (10msec)
    BatchSize 30 # (30KiB)
    # interpreted as {test="fluent-bit-go", lang="Golang"}
    Labels {"labels": [{"key": "test", "label": "fluent-bit-go"},{"key": "lang", "label": "Golang"}]}
```

## Useful links

* [fluent-bit-go](https://github.com/fluent/fluent-bit-go)
