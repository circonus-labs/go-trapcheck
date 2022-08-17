// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/circonus-labs/go-apiclient"
)

func TestNew(t *testing.T) {
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
		cfg     *Config
		want    *TrapCheck
		name    string
		wantErr bool
	}{
		{name: "invalid, nil config", wantErr: true},
		{name: "invalid, no api client", cfg: &Config{}, wantErr: true},
		{
			name: "valid, pre-existing check",
			cfg: &Config{
				CheckConfig: &apiclient.CheckBundle{CID: "/check_bundle/123"},
				Client: &APIMock{
					FetchCheckBundleFunc: func(cid apiclient.CIDType) (*apiclient.CheckBundle, error) {
						return &apiclient.CheckBundle{
							CID:     "/check_bundle/123",
							Brokers: []string{"/broker/123"},
							Type:    "httptrap",
							Config:  apiclient.CheckBundleConfig{"submission_url": fmt.Sprintf("http://%s:%d", brokerIP, brokerPort)},
							Status:  "active",
						}, nil
					},
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
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			// if !reflect.DeepEqual(got, tt.want) {
			// 	t.Errorf("New() = %v, want %v", got, tt.want)
			// }
		})
	}
}

func TestTrapCheck_GetBrokerTLSConfig(t *testing.T) {
	tc := &TrapCheck{
		checkBundle: &apiclient.CheckBundle{
			Config: apiclient.CheckBundleConfig{
				"submission_url": "https://127.0.0.1",
			},
		},
		submissionURL: "https://127.0.0.1",
	}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	tests := []struct {
		brokerTLS *tls.Config
		want      *tls.Config
		name      string
		wantErr   bool
	}{
		{
			name:      "nil",
			brokerTLS: nil,
			want:      nil,
			wantErr:   true,
		},
		{
			name:      "valid",
			brokerTLS: &tls.Config{ServerName: "foobar", MinVersion: tls.VersionTLS12},
			want:      &tls.Config{ServerName: "foobar", MinVersion: tls.VersionTLS12},
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tc.tlsConfig = tt.brokerTLS
			got, err := tc.GetBrokerTLSConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.GetBrokerTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TrapCheck.GetBrokerTLSConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrapCheck_GetCheckBundle(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	tests := []struct {
		bundle  *apiclient.CheckBundle
		name    string
		want    apiclient.CheckBundle
		wantErr bool
	}{
		{
			name:    "nil",
			bundle:  nil,
			want:    apiclient.CheckBundle{},
			wantErr: true,
		},
		{
			name:    "valid",
			bundle:  &apiclient.CheckBundle{CID: "/check_bundle/123"},
			want:    apiclient.CheckBundle{CID: "/check_bundle/123"},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tc.checkBundle = tt.bundle
			got, err := tc.GetCheckBundle()
			if (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.GetCheckBundle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TrapCheck.GetCheckBundle() = %v, want %v", got, tt.want)
			}
		})
	}
}
