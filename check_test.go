// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/circonus-labs/go-apiclient"
	brokerList "github.com/circonus-labs/go-trapcheck/internal/broker_list"
)

func TestTrapCheck_applyCheckBundleDefaults(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	type args struct {
		cfg *apiclient.CheckBundle
	}
	tests := []struct {
		args    args
		name    string
		wantErr bool
	}{
		{
			name:    "basic",
			args:    args{cfg: &apiclient.CheckBundle{Brokers: []string{"/broker/123"}}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := tc.applyCheckBundleDefaults(tt.args.cfg); (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.applyCheckBundleDefaults() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTrapCheck_fetchCheckBundle(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "beep boop")
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("creating test broker: %s", err)
	}
	brokerIP := tsURL.Hostname()
	bp, err := strconv.Atoi(tsURL.Port())
	if err != nil {
		t.Fatalf("parsing test broker port: %s", err)
	}
	brokerPort := uint16(bp)

	basicBrokerClient := &APIMock{
		FetchBrokersFunc: func() (*[]apiclient.Broker, error) {
			return &[]apiclient.Broker{
				{
					CID:  "/broker/123",
					Name: "foo",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
					Tags: []string{"foo:bar"},
				},
				{
					CID:  "/broker/456",
					Name: "bar",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
				},
				{
					CID:  "/broker/789",
					Name: "baz",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
					Tags: []string{"ack:nak", "wing:ding"},
				},
			}, nil
		},
	}

	tests := []struct {
		checkConfig  *apiclient.CheckBundle
		client       API
		brokerClient API
		name         string
		wantErr      bool
	}{
		{
			name:        "invalid, not found",
			checkConfig: &apiclient.CheckBundle{CID: "/check_bundle/123"},
			wantErr:     true,
			client: &APIMock{
				FetchCheckBundleFunc: func(cid apiclient.CIDType) (*apiclient.CheckBundle, error) {
					return nil, fmt.Errorf("API 404 - not found")
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:        "invalid, no submission_url",
			checkConfig: &apiclient.CheckBundle{CID: "/check_bundle/123"},
			wantErr:     true,
			client: &APIMock{
				FetchCheckBundleFunc: func(cid apiclient.CIDType) (*apiclient.CheckBundle, error) {
					return &apiclient.CheckBundle{CID: "/check_bundle/123"}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:        "invalid, broker not found",
			checkConfig: &apiclient.CheckBundle{CID: "/check_bundle/123"},
			wantErr:     true,
			client: &APIMock{
				FetchCheckBundleFunc: func(cid apiclient.CIDType) (*apiclient.CheckBundle, error) {
					return &apiclient.CheckBundle{
						CID:     "/check_bundle/123",
						Config:  apiclient.CheckBundleConfig{"submission_url": "http://127.0.0.1"},
						Brokers: []string{"/broker/123"},
					}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:        "invalid, check not active",
			checkConfig: &apiclient.CheckBundle{CID: "/check_bundle/123"},
			wantErr:     true,
			client: &APIMock{
				FetchCheckBundleFunc: func(cid apiclient.CIDType) (*apiclient.CheckBundle, error) {
					return &apiclient.CheckBundle{
						CID:     "/check_bundle/123",
						Config:  apiclient.CheckBundleConfig{"submission_url": "http://127.0.0.1"},
						Brokers: []string{"/broker/123"},
						Type:    "httptrap:cua:host:linux",
						Status:  "invalid",
					}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:        "valid",
			checkConfig: &apiclient.CheckBundle{CID: "/check_bundle/123"},
			wantErr:     false,
			client: &APIMock{
				FetchCheckBundleFunc: func(cid apiclient.CIDType) (*apiclient.CheckBundle, error) {
					return &apiclient.CheckBundle{
						CID:     "/check_bundle/123",
						Config:  apiclient.CheckBundleConfig{"submission_url": "http://127.0.0.1"},
						Brokers: []string{"/broker/123"},
						Type:    "httptrap:cua:host:linux",
						Status:  statusActive,
					}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := brokerList.Init(tt.brokerClient, tc.Log); err != nil {
				t.Errorf("initializing broker list: %s", err)
			}
			if bl, err := brokerList.GetInstance(); err != nil {
				t.Errorf("getting broker list instance: %s", err)
			} else {
				tc.brokerList = bl
			}
			tc.client = tt.client
			tc.checkConfig = tt.checkConfig
			if err := tc.fetchCheckBundle(); (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.fetchCheckBundle() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTrapCheck_createCheckBundle(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "beep boop")
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("creating test broker: %s", err)
	}
	brokerIP := tsURL.Hostname()
	bp, err := strconv.Atoi(tsURL.Port())
	if err != nil {
		t.Fatalf("parsing test broker port: %s", err)
	}
	brokerPort := uint16(bp)

	basicBrokerClient := &APIMock{
		FetchBrokersFunc: func() (*[]apiclient.Broker, error) {
			return &[]apiclient.Broker{
				{
					CID:  "/broker/123",
					Name: "foo",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
					Tags: []string{"foo:bar"},
				},
				{
					CID:  "/broker/456",
					Name: "bar",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
				},
				{
					CID:  "/broker/789",
					Name: "baz",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
					Tags: []string{"ack:nak", "wing:ding"},
				},
			}, nil
		},
	}

	tests := []struct {
		client       API
		brokerClient API
		cfg          *apiclient.CheckBundle
		name         string
		wantErr      bool
	}{
		{
			name:    "invalid, nil config",
			wantErr: true,
		},
		{
			name:         "invalid config",
			cfg:          &apiclient.CheckBundle{},
			brokerClient: basicBrokerClient,
			client: &APIMock{
				CreateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return nil, fmt.Errorf("API 500 - failure")
				},
			},
			wantErr: true,
		},
		{
			name:         "valid",
			cfg:          &apiclient.CheckBundle{},
			brokerClient: basicBrokerClient,
			client: &APIMock{
				CreateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return &apiclient.CheckBundle{CID: "/check_bundle/123"}, nil
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "valid" {
				if err := tc.applyCheckBundleDefaults(tt.cfg); err != nil {
					t.Fatalf("applying defaults: %s", err)
				}
			}
			tc.client = tt.client
			if err := brokerList.Init(tt.brokerClient, tc.Log); err != nil {
				t.Errorf("initializing broker list: %s", err)
			}
			if bl, err := brokerList.GetInstance(); err != nil {
				t.Errorf("getting broker list instance: %s", err)
			} else {
				tc.brokerList = bl
			}
			if err := tc.createCheckBundle(tt.cfg); (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.createCheckBundle() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTrapCheck_findCheckBundle(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	tests := []struct {
		client     API
		cfg        *apiclient.CheckBundle
		name       string
		searchTags apiclient.TagType
		want       bool
		wantErr    bool
	}{
		{
			name: "invalid, not found",
			cfg: &apiclient.CheckBundle{
				Type:   "httptrap:foo:bar",
				Target: "foobar",
			},
			searchTags: apiclient.TagType{"service:foo"},
			want:       false,
			wantErr:    true,
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return nil, fmt.Errorf("API 404 - not found")
				},
			},
		},
		{
			name: "invalid, multiple bundles 0 with matching type",
			cfg: &apiclient.CheckBundle{
				Type:   "httptrap:foo:bar",
				Target: "foobar",
			},
			searchTags: apiclient.TagType{"service:foo"},
			want:       false,
			wantErr:    true,
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{
						{CID: "/check_bundle/123", Type: "foo"},
						{CID: "/check_bundle/123", Type: "bar"},
					}, nil
				},
			},
		},
		{
			name: "invalid, multiple bundles >1 with matching type",
			cfg: &apiclient.CheckBundle{
				Type:   "httptrap:foo:bar",
				Target: "foobar",
			},
			searchTags: apiclient.TagType{"service:foo"},
			want:       false,
			wantErr:    true,
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{
						{CID: "/check_bundle/123", Type: "httptrap:foo:bar"},
						{CID: "/check_bundle/123", Type: "httptrap:foo:bar"},
					}, nil
				},
			},
		},
		{
			name: "valid, multiple bundles 1 with matching type",
			cfg: &apiclient.CheckBundle{
				Type:   "httptrap:foo:bar",
				Target: "foobar",
			},
			searchTags: apiclient.TagType{"service:foo"},
			want:       true,
			wantErr:    false,
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{
						{CID: "/check_bundle/123", Type: "httptrap:foo:bar"},
						{CID: "/check_bundle/123", Type: "bar"},
					}, nil
				},
			},
		},
		{
			name: "valid, one bundle found",
			cfg: &apiclient.CheckBundle{
				Type:   "httptrap:foo:bar",
				Target: "foobar",
			},
			searchTags: apiclient.TagType{"service:foo"},
			want:       true,
			wantErr:    false,
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{
						{CID: "/check_bundle/123", Type: "httptrap:foo:bar"},
					}, nil
				},
			},
		},
		{
			name: "valid, no bundle found -- trigger check create",
			cfg: &apiclient.CheckBundle{
				Type:   "httptrap:foo:bar",
				Target: "foobar",
			},
			searchTags: apiclient.TagType{"service:foo"},
			want:       false,
			wantErr:    false,
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{}, nil
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tc.client = tt.client
			tc.checkSearchTags = tt.searchTags
			got, err := tc.findCheckBundle(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.findCheckBundle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TrapCheck.findCheckBundle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrapCheck_initCheckBundle(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "beep boop")
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("creating test broker: %s", err)
	}
	brokerIP := tsURL.Hostname()
	bp, err := strconv.Atoi(tsURL.Port())
	if err != nil {
		t.Fatalf("parsing test broker port: %s", err)
	}
	brokerPort := uint16(bp)

	basicBrokerClient := &APIMock{
		FetchBrokersFunc: func() (*[]apiclient.Broker, error) {
			return &[]apiclient.Broker{
				{
					CID:  "/broker/123",
					Name: "foo",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
					Tags: []string{"foo:bar"},
				},
				{
					CID:  "/broker/456",
					Name: "bar",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
				},
				{
					CID:  "/broker/789",
					Name: "baz",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
					Tags: []string{"ack:nak", "wing:ding"},
				},
			}, nil
		},
	}

	tests := []struct {
		client          API
		brokerClient    API
		cfg             *apiclient.CheckBundle
		name            string
		checkSearchTags apiclient.TagType
		wantErr         bool
	}{
		{
			name:            "search error",
			cfg:             &apiclient.CheckBundle{Type: "httptrap:foo:bar", Target: "foobar"},
			checkSearchTags: apiclient.TagType{"service:foobar"},
			wantErr:         true,
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return nil, fmt.Errorf("API 404 - not found")
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:            "success: search found",
			cfg:             &apiclient.CheckBundle{Type: "httptrap:foo:bar", Target: "foobar"},
			checkSearchTags: apiclient.TagType{"service:foobar"},
			wantErr:         false,
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{
						{CID: "/check_bundle/123", Type: "httptrap:foo:bar"},
					}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:            "search not found, create error",
			cfg:             &apiclient.CheckBundle{Type: "httptrap:foo:bar", Target: "foobar"},
			checkSearchTags: apiclient.TagType{"service:foobar"},
			wantErr:         true,
			client: &APIMock{
				CreateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return nil, fmt.Errorf("API 500 - failure")
				},
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:            "success: search not found, create",
			cfg:             &apiclient.CheckBundle{Type: "httptrap:foo:bar", Target: "foobar"},
			checkSearchTags: apiclient.TagType{"service:foobar"},
			wantErr:         false,
			client: &APIMock{
				CreateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return &apiclient.CheckBundle{CID: "/check_bundle/123"}, nil
				},
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tc.client = tt.client
			if err := brokerList.Init(tt.brokerClient, tc.Log); err != nil {
				t.Errorf("initializing broker list: %s", err)
			}
			if bl, err := brokerList.GetInstance(); err != nil {
				t.Errorf("getting broker list instance: %s", err)
			} else {
				tc.brokerList = bl
			}
			tc.checkSearchTags = tt.checkSearchTags
			if err := tc.initCheckBundle(tt.cfg); (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.initCheckBundle() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTrapCheck_initializeCheck(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "beep boop")
	}))
	defer ts.Close()

	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("creating test broker: %s", err)
	}
	brokerIP := tsURL.Hostname()
	bp, err := strconv.Atoi(tsURL.Port())
	if err != nil {
		t.Fatalf("parsing test broker port: %s", err)
	}
	brokerPort := uint16(bp)

	basicBrokerClient := &APIMock{
		FetchBrokersFunc: func() (*[]apiclient.Broker, error) {
			return &[]apiclient.Broker{
				{
					CID:  "/broker/123",
					Name: "foo",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
					Tags: []string{"foo:bar"},
				},
				{
					CID:  "/broker/456",
					Name: "bar",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
				},
				{
					CID:  "/broker/789",
					Name: "baz",
					Type: circonusType,
					Details: []apiclient.BrokerDetail{
						{
							Status:  statusActive,
							Modules: []string{"httptrap"},
							IP:      &brokerIP,
							Port:    &brokerPort,
						},
					},
					Tags: []string{"ack:nak", "wing:ding"},
				},
			}, nil
		},
	}

	tests := []struct {
		client          API
		brokerClient    API
		checkConfig     *apiclient.CheckBundle
		name            string
		checkSearchTags apiclient.TagType
		wantErr         bool
	}{
		{
			name:    "cfg w/cid - check - api error",
			wantErr: true,
			checkConfig: &apiclient.CheckBundle{
				CID:     "/check_bundle/123",
				Brokers: []string{"/broker/123"},
				Type:    "httptrap",
				Status:  statusActive,
			},
			client: &APIMock{
				FetchCheckBundleFunc: func(cid apiclient.CIDType) (*apiclient.CheckBundle, error) {
					return nil, fmt.Errorf("API 404 - not found")
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:    "cfg w/cid - broker - api error",
			wantErr: true,
			checkConfig: &apiclient.CheckBundle{
				CID:     "/check_bundle/123",
				Brokers: []string{"/broker/123"},
				Type:    "httptrap",
				Status:  statusActive,
			},
			client: &APIMock{
				FetchCheckBundleFunc: func(cid apiclient.CIDType) (*apiclient.CheckBundle, error) {
					return &apiclient.CheckBundle{CID: "/check_bundle/123", Brokers: []string{"/broker/123"}}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:        "success: cfg w/cid",
			wantErr:     false,
			checkConfig: &apiclient.CheckBundle{CID: "/check_bundle/123", Brokers: []string{"/broker/123"}},
			client: &APIMock{
				FetchCheckBundleFunc: func(cid apiclient.CIDType) (*apiclient.CheckBundle, error) {
					return &apiclient.CheckBundle{
						CID:     "/check_bundle/123",
						Brokers: []string{"/broker/123"},
						Type:    "httptrap",
						Config:  apiclient.CheckBundleConfig{"submission_url": fmt.Sprintf("http://%s:%d", brokerIP, brokerPort)},
						Status:  statusActive,
					}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:            "success: search",
			wantErr:         false,
			checkSearchTags: apiclient.TagType{"service:foo"},
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{
						{
							CID:     "/check_bundle/123",
							Type:    "httptrap:foo:bar",
							Brokers: []string{"/broker/123"},
							Config:  apiclient.CheckBundleConfig{"submission_url": fmt.Sprintf("http://%s:%d", brokerIP, brokerPort)},
							Status:  statusActive,
						},
					}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
		{
			name:            "success: create",
			wantErr:         false,
			checkSearchTags: apiclient.TagType{"service:foo"},
			client: &APIMock{
				SearchCheckBundlesFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error) {
					return &[]apiclient.CheckBundle{}, nil
				},
				CreateCheckBundleFunc: func(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error) {
					return &apiclient.CheckBundle{
						CID:     "/check_bundle/123",
						Type:    "httptrap:foo:bar",
						Brokers: []string{"/broker/123"},
						Config:  apiclient.CheckBundleConfig{"submission_url": fmt.Sprintf("http://%s:%d", brokerIP, brokerPort)},
						Status:  statusActive,
					}, nil
				},
			},
			brokerClient: basicBrokerClient,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tc.client = tt.client
			if err := brokerList.Init(tt.brokerClient, tc.Log); err != nil {
				t.Errorf("initializing broker list: %s", err)
			}
			if bl, err := brokerList.GetInstance(); err != nil {
				t.Errorf("getting broker list instance: %s", err)
			} else {
				tc.brokerList = bl
			}
			tc.checkConfig = tt.checkConfig
			tc.checkSearchTags = tt.checkSearchTags
			if err := tc.initializeCheck(); (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.initializeCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
