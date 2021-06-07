# Package `go-trapcheck`

HTTPTrap check management to separate checkmgr from cgm. Goes with `go-trapmetrics` for handling collection and submission of metrics from applications.

## Configuration options

* Client - required, an instance of the [API Client](https://github.com/circonus-labs/go-apiclient)
* CheckConfig - optional, pointer to a valid [API Client Check Bundle](https://pkg.go.dev/github.com/circonus-labs/go-apiclient#CheckBundle). If it is used at all, some or none of the settings may be used, offering the most flexible method for configuring a check bundle to be created. Pass `nil` for the defaults. Defaults will be used to backfill any partial configuration used. (e.g. set the Target and all other settings will use defaults.)
* SubmissionURL - optional, explicit submission URL to use when sending metrics (e.g. a circonus-agent on the local host). If the destination is using TLS then a `SubmitTLSConfig` must be provided.
* SubmitTLSConfig - optional, pointer to a valid `tls.Config` for the submission target (e.g. the broker or an explicit submission URL using TLS).
* Logger - optional, something satisfying the Logger interface defined in this module.
* BrokerSelectTags - optional, when creating a check and the check configuraiton does not contain an explict broker, one will be selected. These tags provide a way to define which broker(s) should be evaluated.
* BrokerMaxResponseTiime - optional, duration defining in what time the broker must respond to a connection in order to be considered valid for selection when creating a new check.
* CheckSearchTags - optional, the module will first search for an existing check satisfying certain conditions (active, check type, check target). This setting provides a method for narrowing the search down more explicitly for checks created via other mechanisms.
* TraceMetrics - optional, the path where metric traces should be written. Each metric submission (raw JSON) will be written to a file in this location. If set to `-`, metrics will be written using `Logger.Infof()`. Infof is used so that regular debug messages and tracing can be controlled independently. For debugging purposes only.

## Basic pseudocode example

```go
package main

import (
    "log"

    apiclient "github.com/circonus-labs/go-apiclient"
    trapcheck "github.com/circonus-labs/go-trapcheck"
)

func main() {
    logger := log.New(os.Stderr, "", log.LstdFlags)

    client, err := apiclient.New(&apiclient.Config{
        TokenKey: "", // required, Circonus API Token Key
        // any other API Client settings desired
    })
    if err != nil {
        logger.Fatal(err)
    }

    
    check, err := trapcheck.New(&trapcheck.Config{
        Client: client, // required, Client is a valid circonus go-apiclient instance
        // CheckConfig is a valid circonus apiclient.CheckBundle configuration
        // or nil for defaults
        // CheckConfig: &apiclient.CheckBundle{...},
        // SubmissionURL explicit submission url (e.g. submitting to an agent, 
        // if tls used a SubmitTLSConfig is required)
        // SubmissionURL: "",
        // SubmitTLSConfig is a *tls.Config to use when submitting to the broker
        // SubmitTLSConfig: &tls.Config{...},
        // Logger interface for logging
        Logger: &trapcheck.LogWrapper{
            Log:   logger,
            Debug: false,
        },
        // BrokerSelectTags defines tags to use when selecting a broker to use (when creating a check)
        // BrokerSelectTag: apiclient.TagType{"location:us_east"},
        // BrokerMaxResponseTime defines the timeout in which brokers must respond when selecting
        // BrokerMaxResponseTime: "500ms",
        // CheckSearchTags defines tags to use when searching for a check
        // CheckSearchTag: apiclient.TagType{"service_id:web21"},
        // TraceMetrics path to write traced metrics to (must be writable by the user running app)
        // TraceMetrics: "/tmp/metric_trace",
    })
    if err != nil {
        logger.Fatal(err)
    }
}
```

---

Unless otherwise noted, the source files are distributed under the BSD-style license found in the [LICENSE](LICENSE) file.
