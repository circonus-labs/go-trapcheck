# v0.0.10

* feat: add gzip size of sent data
* feat: singleton broker list [CIRC-9594]

# v0.0.9

* chore: add RefreshCheckBundle tests
* feet: add `RefreshCheckBundle` method
* chore: add UpdateCheckTags tests
* feat: add `UpdateCheckTags` method
* fix: ensure the trap check has been initialized
* feat(deps): bump go-apiclient from 0.7.15 to 0.7.18
* feat(deps): bump go-retryablehttp from 0.7.0 to 0.7.1
* fix(lint): ioutil deprecation
* chore: update to go1.17

# v0.0.8

* add: reset flag and handle rest in setBrokerTLSConfig
* upd: skip conn test if check type is httptrap and using proxy env vars HTTP_PROXY and/or HTTPS_PROXY
* upd: move cert error logging to tls config
* upd: disable gci

# v0.0.7

* add: tracking if bundle is new (created) or not
* upd: only, allow rule, so a deny is not evaluated by broker for every incoming metric. one rule _must_ be provided in order to enable metric_filters
* upd: use bytes.Buffer for metrics
* add: reader for seek in order to be able to trace (io.Copy drains a buffer leaving it at EOF)
* add: public broker ca setting
* add: exposure of whether this is a new (created) check
* upd: use bytes.Buffer for metrics
* upd: clarity around refreshing check on errors
* upd: GetCheckBundle returns the bundle not a ptr
* upd: add public broker ca setting awareness
* upd: ignore generated mocks

# v0.0.6

* upd: only use an allow rule in metric filter when creating a new check to reduce load on broker processing

# v0.0.5

* add: NewFromCheckBundle to handle init from explicit check bundle (e.g. cached)

# v0.0.4

* fix: peer cert verify bad return on nil err

# v0.0.3

* fix: reduce log message size when broker responds with 406

# v0.0.2

* add: dependabot config
* fix: lint issues
* add: lint config

# v0.0.1

* initial
