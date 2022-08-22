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
)

func TestTrapCheck_brokerSupportsCheckType(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	type args struct {
		details   *apiclient.BrokerDetail
		checkType string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name:    "invalid, nil details",
			args:    args{details: nil, checkType: ""},
			want:    false,
			wantErr: true,
		},
		{
			name:    "invalid, empty check type",
			args:    args{details: &apiclient.BrokerDetail{}, checkType: ""},
			want:    false,
			wantErr: true,
		},
		{
			name:    "invalid, unknown check type",
			args:    args{details: &apiclient.BrokerDetail{Modules: []string{"foo", "bar"}}, checkType: "blah"},
			want:    false,
			wantErr: true,
		},
		{
			name:    "valid base type",
			args:    args{details: &apiclient.BrokerDetail{Modules: []string{"httptrap"}}, checkType: "httptrap"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "valid complex type",
			args:    args{details: &apiclient.BrokerDetail{Modules: []string{"httptrap"}}, checkType: "httptrap:cua:agent:linux"},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := tc.brokerSupportsCheckType(tt.args.checkType, tt.args.details)
			if (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.brokerSupportsCheckType() error = %v, wantErr %v", got, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("TrapCheck.brokerSupportsCheckType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrapCheck_getBrokerCNList(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	brokerIP := "127.0.0.1"
	brokerPort := uint16(1234)

	tests := []struct {
		name        string
		checkBundle *apiclient.CheckBundle
		broker      *apiclient.Broker
		want        string
		want1       string
		wantErr     bool
	}{
		{
			name:    "invalid, nil broker",
			want:    "",
			want1:   "",
			wantErr: true,
		},
		{
			name: "invalid, nil check",
			broker: &apiclient.Broker{
				Details: []apiclient.BrokerDetail{
					{CN: "foo", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
					{CN: "bar", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
				},
			},
			want:    "",
			want1:   "",
			wantErr: true,
		},
		{
			name: "invalid submission url",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": ":foo",
				},
			},
			broker: &apiclient.Broker{
				Details: []apiclient.BrokerDetail{
					{CN: "foo", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
					{CN: "bar", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
				},
			},
			want:    "",
			want1:   "",
			wantErr: true,
		},
		{
			name: "invalid (no matches)",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": "http://127.0.0.2",
				},
			},
			broker: &apiclient.Broker{
				Details: []apiclient.BrokerDetail{
					{CN: "foo", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
					{CN: "bar", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
				},
			},
			want:    "",
			want1:   "",
			wantErr: true,
		},
		{
			name: "valid (non-ip)",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": "https://api.circonus.com/",
				},
			},
			broker: &apiclient.Broker{
				Details: []apiclient.BrokerDetail{
					{CN: "foo", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
					{CN: "bar", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
				},
			},
			want:    "api.circonus.com",
			want1:   "",
			wantErr: false,
		},
		{
			name: "valid",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": fmt.Sprintf("https://%s:%d", brokerIP, brokerPort),
				},
			},
			broker: &apiclient.Broker{
				Details: []apiclient.BrokerDetail{
					{CN: "foo", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
					{CN: "bar", ExternalHost: &brokerIP, ExternalPort: brokerPort, Status: statusActive},
				},
			},
			want:    "foo",
			want1:   "foo,bar",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tc.broker = tt.broker
			tc.checkBundle = tt.checkBundle
			got, got1, err := tc.getBrokerCNList()
			if (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.getBrokerCNList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TrapCheck.getBrokerCNList() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("TrapCheck.getBrokerCNList() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestTrapCheck_isValidBroker(t *testing.T) {
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

	type args struct {
		broker    *apiclient.Broker
		checkType string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name:    "invalid, nil broker",
			args:    args{broker: nil, checkType: "httptrap"},
			want:    false,
			wantErr: true,
		},
		{
			name:    "invalid broker type",
			args:    args{broker: &apiclient.Broker{Name: "foo", Type: "unknown"}, checkType: "httptrap"},
			want:    false,
			wantErr: true,
		},
		{
			name:    "invalid no details",
			args:    args{broker: &apiclient.Broker{Name: "foo", Type: circonusType}, checkType: "httptrap"},
			want:    false,
			wantErr: true,
		},
		{
			name: "invalid no details w/active status",
			args: args{broker: &apiclient.Broker{
				Name: "foo",
				Type: circonusType,
				Details: []apiclient.BrokerDetail{
					{Status: "bar"},
				},
			}, checkType: "httptrap"},
			want:    false,
			wantErr: true,
		},
		{
			name: "invalid no details w/check type",
			args: args{broker: &apiclient.Broker{
				Name: "foo",
				Type: circonusType,
				Details: []apiclient.BrokerDetail{
					{
						Status:  statusActive,
						Modules: []string{"foo"},
					},
				},
			}, checkType: "httptrap"},
			want:    false,
			wantErr: true,
		},
		{
			name: "invalid no ip or external_host",
			args: args{broker: &apiclient.Broker{
				Name: "foo",
				Type: circonusType,
				Details: []apiclient.BrokerDetail{
					{
						Status:  statusActive,
						Modules: []string{"httptrap"},
					},
				},
			}, checkType: "httptrap"},
			want:    false,
			wantErr: true,
		},
		{
			name: "valid",
			args: args{broker: &apiclient.Broker{
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
			}, checkType: "httptrap"},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := tc.isValidBroker(tt.args.broker, tt.args.checkType)
			if (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.isValidBroker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("TrapCheck.isValidBroker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrapCheck_getBroker(t *testing.T) {
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

	tests := []struct {
		checkConfig      *apiclient.CheckBundle
		checkBundle      *apiclient.CheckBundle
		broker           *apiclient.Broker
		client           API
		name             string
		checkType        string
		wantBrokerType   string
		brokerSelectTags apiclient.TagType
		wantErr          bool
		checkBrokerType  bool
	}{
		{
			name: "invalid (non-existent) broker in passed config",
			client: &APIMock{
				FetchBrokerFunc: func(cid apiclient.CIDType) (*apiclient.Broker, error) {
					return nil, fmt.Errorf("API 404 - broker not found")
				},
			},
			checkConfig: &apiclient.CheckBundle{Brokers: []string{"/broker/123"}},
			checkType:   "httptrap",
			wantErr:     true,
		},
		{
			name: "valid broker in passed config",
			client: &APIMock{
				FetchBrokerFunc: func(cid apiclient.CIDType) (*apiclient.Broker, error) {
					return &apiclient.Broker{
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
					}, nil
				},
			},
			checkConfig: &apiclient.CheckBundle{Brokers: []string{"/broker/123"}},
			checkType:   "httptrap",
			wantErr:     false,
		},
		{
			name: "invalid search broker w/select tag - not found",
			client: &APIMock{
				SearchBrokersFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.Broker, error) {
					return nil, fmt.Errorf("API 404 - broker not found")
				},
			},
			brokerSelectTags: apiclient.TagType{"foo:bar"},
			checkType:        "httptrap",
			wantErr:          true,
		},
		{
			name: "valid search broker w/select tag",
			client: &APIMock{
				SearchBrokersFunc: func(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.Broker, error) {
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
						},
					}, nil
				},
			},
			brokerSelectTags: apiclient.TagType{"foo:bar"},
			checkType:        "httptrap",
			wantErr:          false,
		},
		{
			name: "invalid no brokers found",
			client: &APIMock{
				FetchBrokersFunc: func() (*[]apiclient.Broker, error) {
					return nil, fmt.Errorf("API 404 - broker not found")
				},
			},
			checkType: "httptrap",
			wantErr:   true,
		},
		{
			name: "invalid empty broker list",
			client: &APIMock{
				FetchBrokersFunc: func() (*[]apiclient.Broker, error) {
					return &[]apiclient.Broker{}, nil
				},
			},
			checkType: "httptrap",
			wantErr:   true,
		},
		{
			name: "invalid no active brokers",
			client: &APIMock{
				FetchBrokersFunc: func() (*[]apiclient.Broker, error) {
					return &[]apiclient.Broker{
						{
							CID:  "/broker/123",
							Name: "foo",
							Type: circonusType,
							Details: []apiclient.BrokerDetail{
								{
									Status:  "foo",
									Modules: []string{"httptrap"},
									IP:      &brokerIP,
									Port:    &brokerPort,
								},
							},
						},
						{
							CID:  "/broker/456",
							Name: "bar",
							Type: circonusType,
							Details: []apiclient.BrokerDetail{
								{
									Status:  "bar",
									Modules: []string{"httptrap"},
									IP:      &brokerIP,
									Port:    &brokerPort,
								},
							},
						},
					}, nil
				},
			},
			checkType: "httptrap",
			wantErr:   true,
		},
		{
			name: "valid active brokers",
			client: &APIMock{
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
					}, nil
				},
			},
			checkType: "httptrap",
			wantErr:   false,
		},
		{
			name: "valid active brokers",
			client: &APIMock{
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
						},
						{
							CID:  "/broker/456",
							Name: "bar",
							Type: enterpriseType,
							Details: []apiclient.BrokerDetail{
								{
									Status:  statusActive,
									Modules: []string{"httptrap"},
									IP:      &brokerIP,
									Port:    &brokerPort,
								},
							},
						},
					}, nil
				},
			},
			checkType:       "httptrap",
			wantErr:         false,
			checkBrokerType: true,
			wantBrokerType:  enterpriseType,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tc.client = tt.client
			tc.checkConfig = tt.checkConfig
			tc.checkBundle = tt.checkBundle
			tc.brokerSelectTags = tt.brokerSelectTags
			tc.broker = tt.broker
			if err := tc.getBroker(tt.checkType); (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.getBroker() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.checkBrokerType {
				if tc.broker.Type != tt.wantBrokerType {
					t.Errorf("TrapCheck.getBroker() type = %s, wantBrokerType %s", tc.broker.Type, tt.wantBrokerType)
				}
			}
		})
	}
}
