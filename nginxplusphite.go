package main

import (
  "encoding/json"
  "flag"
  "fmt"
  "log"
  "net/http"
  "os"
  "time"
  "reflect"
  "strings"
  "strconv"

  "github.com/quipo/statsd"
)

var leafKeysToIgnore []string = []string{"version", "generation", "load_timestamp", "timestamp", "pid", "upstream"}
var topKeysToIgnore  []string = []string{"upstreams"}

var (
  host       string
  port       int
  metricPath string
  interval   int
  url        string
  version    int
)

func init() {
  flag.StringVar(&host, "H", "localhost", "Hostname for statsd")
  flag.IntVar(&port, "p", 8125, "Port for statsd")
  flag.StringVar(&metricPath, "m", "nginx.stats", "Metric path")
  flag.IntVar(&interval, "i", 10, "Check stats each <i> seconds")
  flag.StringVar(&url, "u", "http://localhost/status", "Nginx plus status URL")
  flag.IntVar(&version, "v", 5, "NGinx JSON version")
}

func main() {

  tenSecs := time.Duration(interval) * time.Second

  if len(os.Args) < 4 {
    flag.Usage()
    os.Exit(127)
  }
  flag.Parse()

  c := statsd.NewStatsdClient(fmt.Sprintf("%s:%d", host, port), metricPath)

  for {
    log.Printf("Running...")
    work(c)
    log.Printf("Done! Sleeping for %d seconds", interval)
    time.Sleep(tenSecs)
  }

}

func ToInt64(num json.Number) int64 {
  numInt, err := num.Int64()
  if err != nil {
    log.Printf("Error converting json.Number to int64")
  }
  return numInt
}

func BuildPath(base string, addition string) string {
  return fmt.Sprintf("%s%s.", base, strings.Replace(addition, ".", "-", -1))
}

func BoolToInt(b bool) int64 {
  if b {
      return 1
  }
  return 0
}

func StringInSlice(a string, list []string) bool {
    for _, b := range list {
        if b == a {
            return true
        }
    }
    return false
}

func TrimSuffix(s, suffix string) string {
  if strings.HasSuffix(s, suffix) {
      s = s[:len(s)-len(suffix)]
  }
  return s
}

func IterateMap(c *statsd.StatsdClient, b map[string]interface{}, path string) {
  for key, value := range b {
  	if StringInSlice(key, topKeysToIgnore) {
  		continue
  	}
    fp := BuildPath(path, key)
    switch reflect.ValueOf(value).Kind() {
      case reflect.Map:
        IterateMap(c, value.(map[string]interface{}), fp)
      case reflect.Slice:
        IterateArray(c, value.([]interface{}), fp)
      case reflect.String:
        switch value.(type) {
        case json.Number:
          if !StringInSlice(key, leafKeysToIgnore) {
            ToStatsd(c, fp, ToInt64(value.(json.Number)))
          }
        default:
          // statsd doesn't accept strings
          continue
        }
      case reflect.Bool:
        ToStatsd(c, path, BoolToInt(value.(bool)))
      default:
        log.Printf("Unexpected type %s KEY: %s\n", reflect.TypeOf(value).Kind(), key)
    }
  }
}

func IterateArray(c *statsd.StatsdClient, b []interface{}, path string) {
	for i := 0; i < len(b); i+= 1 {
		fp := BuildPath(path, strconv.FormatInt(int64(i), 10))
    switch reflect.ValueOf(b[i]).Kind() {
    case reflect.Map:
      IterateMap(c, b[i].(map[string]interface{}), fp)
    case reflect.Slice:
      IterateArray(c, b[i].([]interface{}), fp)
    default:
      log.Printf("Unexpected type %s KEY: %s\n", reflect.TypeOf(b[i]).Kind(), b[i])
    }
  }
}

func ToStatsd(c *statsd.StatsdClient, path string, value int64) {
  c.Gauge(TrimSuffix(path, "."), value)
}

func work(c *statsd.StatsdClient) {
  err := c.CreateSocket()
  if err != nil {
    log.Fatal("Error creating socket")
  }

  defer c.Close()

  // Read results from status URL
  resp, err := http.Get(url)
  defer resp.Body.Close()
  if err != nil {
    log.Fatalf("%v", err)
  }

  m := map[string]interface{}{}
  d := json.NewDecoder(resp.Body)
  d.UseNumber()
  err = d.Decode(&m)
  if err != nil {
    log.Fatalf("%v", err)
  }

  IterateMap(c, m, "")
}
