package main

import (
	"C"
	"os"
	"time"
	"unsafe"
  "fmt"
  "strings"

	"github.com/cortexproject/cortex/pkg/util/flagext"
	"github.com/fluent/fluent-bit-go/output"
	"github.com/go-kit/kit/log/level"
	kit "github.com/go-kit/kit/log/logrus"
	"github.com/grafana/loki/pkg/promtail/client"
	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/sirupsen/logrus"
)

var loki client.Client
var ls model.LabelSet
var al string
var plugin GoOutputPlugin = &fluentPlugin{}
var logger = defaultLogger()

type GoOutputPlugin interface {
	PluginConfigKey(ctx unsafe.Pointer, key string) string
	Unregister(ctx unsafe.Pointer)
	GetRecord(dec *output.FLBDecoder) (ret int, ts interface{}, rec map[interface{}]interface{})
	NewDecoder(data unsafe.Pointer, length int) *output.FLBDecoder
	HandleLine(ls model.LabelSet, timestamp time.Time, line string) error
	Exit(code int)
}

type fluentPlugin struct{}

func (p *fluentPlugin) PluginConfigKey(ctx unsafe.Pointer, key string) string {
	return output.FLBPluginConfigKey(ctx, key)
}

func (p *fluentPlugin) Unregister(ctx unsafe.Pointer) {
	output.FLBPluginUnregister(ctx)
}

func (p *fluentPlugin) GetRecord(dec *output.FLBDecoder) (int, interface{}, map[interface{}]interface{}) {
	return output.GetRecord(dec)
}

func (p *fluentPlugin) NewDecoder(data unsafe.Pointer, length int) *output.FLBDecoder {
	return output.NewDecoder(data, length)
}

func (p *fluentPlugin) Exit(code int) {
	os.Exit(code)
}

func (p *fluentPlugin) HandleLine(ls model.LabelSet, timestamp time.Time, line string) error {
	return loki.Handle(ls, timestamp, line)
}

//export FLBPluginRegister
func FLBPluginRegister(ctx unsafe.Pointer) int {
	return output.FLBPluginRegister(ctx, "loki", "Loki Output plugin written in GO!")
}

//export FLBPluginInit
// (fluentbit will call this)
// ctx (context) pointer to fluentbit context (state/ c code)
func FLBPluginInit(ctx unsafe.Pointer) int {
	// Example to retrieve an optional configuration parameter
	url := plugin.PluginConfigKey(ctx, "URL")
	batchWait := plugin.PluginConfigKey(ctx, "BatchWait")
	batchSize := plugin.PluginConfigKey(ctx, "BatchSize")
	labels := plugin.PluginConfigKey(ctx, "Labels")
	autoLabel := plugin.PluginConfigKey(ctx, "AutoLabel")
	logLevel := plugin.PluginConfigKey(ctx, "LogLevel")

	config, err := getLokiConfig(url, batchWait, batchSize, labels, autoLabel, logLevel)
	if err != nil {
		level.Error(logger).Log("[flb-go]", "failed to launch", "error", err)
		plugin.Unregister(ctx)
		plugin.Exit(1)
		return output.FLB_ERROR
	}
	logger = newLogger(config.logLevel)
	level.Info(logger).Log("[flb-go]", "Starting fluent-bit-go-loki", "version", version.Info())
	level.Info(logger).Log("[flb-go]", "provided parameter", "URL", url)
	level.Info(logger).Log("[flb-go]", "provided parameter", "BatchWait", batchWait)
	level.Info(logger).Log("[flb-go]", "provided parameter", "BatchSize", batchSize)
	level.Info(logger).Log("[flb-go]", "provided parameter", "Labels", labels)
	level.Info(logger).Log("[flb-go]", "provided parameter", "AutoLabel", autoLabel)
	level.Info(logger).Log("[flb-go]", "provided parameter", "LogLevel", logLevel)

	cfg := client.Config{}
	// Init everything with default values.
	flagext.RegisterFlags(&cfg)

	// Override some of those defaults
	cfg.URL = config.url
	cfg.BatchWait = config.batchWait
	cfg.BatchSize = config.batchSize

	log := logrus.New()

	loki, err = client.New(cfg, kit.NewLogrusLogger(log))
	if err != nil {
		level.Error(logger).Log("client.New", err)
		plugin.Unregister(ctx)
		plugin.Exit(1)
		return output.FLB_ERROR
	}
	ls = config.labelSet
  al = config.autoLabel

	return output.FLB_OK
}

//export FLBPluginFlush
func FLBPluginFlush(data unsafe.Pointer, length C.int, tag *C.char) int {
	var ret int
	var ts interface{}
	var record map[interface{}]interface{}

	dec := plugin.NewDecoder(data, int(length))

	for {
		ret, ts, record = plugin.GetRecord(dec)
		if ret != 0 {
			break
		}

		// Get timestamp
		var timestamp time.Time
		switch t := ts.(type) {
		case output.FLBTime:
			timestamp = ts.(output.FLBTime).Time
		case uint64:
			timestamp = time.Unix(int64(t), 0)
		default:
			level.Warn(logger).Log("msg", "timestamp isn't known format. Use current time.")
			timestamp = time.Now()
		}

		linelabel, line, err := createJSON(al, record)
		if err != nil {
			level.Error(logger).Log("msg", "error creating message for Grafana Loki", "error", err)
			continue
		}

		err = plugin.HandleLine(linelabel, timestamp, line)
		if err != nil {
			level.Error(logger).Log("msg", "error sending message for Grafana Loki", "error", err)
			return output.FLB_ERROR
		}
	}

	// Return options:
	//
	// output.FLB_OK    = data have been processed.
	// output.FLB_ERROR = unrecoverable error, do not try this again.
	// output.FLB_RETRY = retry to flush later.
	return output.FLB_OK
}

func createJSON(labelkey string, record map[interface{}]interface{}) (model.LabelSet, string, error) {
  labelSet := make(model.LabelSet)
	m := make(map[string]interface{})
  replacer := strings.NewReplacer("/", "_", ".", "_", "-", "_")

	for k, v := range record {
    if k.(string) == labelkey {
      for g, h := range v.(map[interface{}]interface{}) {
        if g.(string) == "labels" {
          for c, d := range h.(map[interface{}]interface{}) {
			      switch t := d.(type) {
			      case []byte:
              labelSet[model.LabelName(replacer.Replace(c.(string)))] = model.LabelValue(string(t))
			      default:
              labelSet[model.LabelName(replacer.Replace(c.(string)))] = model.LabelValue(fmt.Sprintf("%v",d))
	          }
          }
        } else if g.(string) == "docker_id" || g.(string) == "pod_id" || g.(string) == "annotations" {
          // do nothing
        } else {
			    switch t := h.(type) {
			    case []byte:
            labelSet[model.LabelName(g.(string))] = model.LabelValue(string(t))
			    default:
            labelSet[model.LabelName(g.(string))] = model.LabelValue(fmt.Sprintf("%v",h))
	        }
        }
      }
		} else {
			switch t := v.(type) {
			case []byte:
				// prevent encoding to base64
				m[k.(string)] = string(t)
			default:
				m[k.(string)] = v
	    }
		}
	}

	js, err := jsoniter.Marshal(m)
	if err != nil {
		return ls, "{}", err
	}

  if len(labelSet) > 0 {
    return labelSet, string(js), nil
  } else {
	  return ls, string(js), nil
  }
}

//export FLBPluginExit
func FLBPluginExit() int {
	loki.Stop()
	return output.FLB_OK
}

func main() {
}
