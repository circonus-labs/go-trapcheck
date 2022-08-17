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
	"reflect"
	"testing"

	"github.com/circonus-labs/go-apiclient"
	"github.com/circonus-labs/go-apiclient/config"
)

func TestTrapCheck_fetchCert(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	tests := []struct {
		client  API
		name    string
		want    []byte
		wantErr bool
	}{
		{
			name: "error from api",
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return nil, fmt.Errorf("api error")
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid json",
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return []byte(""), nil
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid json, no Contents",
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return []byte(`{}`), nil
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "valid",
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return []byte(`{"Contents":"foobar"}`), nil
				},
			},
			want:    []byte(`foobar`),
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tc.client = tt.client
			got, err := tc.fetchCert()
			if (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.fetchCert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("TrapCheck.fetchCert() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTrapCheck_setBrokerTLSConfig(t *testing.T) {
	tc := &TrapCheck{}
	tc.Log = &LogWrapper{
		Log:   log.New(io.Discard, "", log.LstdFlags),
		Debug: false,
	}

	brokerIP := "127.0.0.1"
	brokerPort := uint16(1234)

	tests := []struct {
		client      API
		broker      *apiclient.Broker
		checkBundle *apiclient.CheckBundle
		tlsConfig   *tls.Config
		name        string
		wantErr     bool
	}{
		{
			name:      "already have tlsconfig",
			tlsConfig: &tls.Config{ServerName: "foobar"},
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return circCA, nil
				},
			},
			wantErr: false,
		},
		{
			name: "invalid, submission url",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": ":foobar",
				},
			},
			tlsConfig: nil,
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return circCA, nil
				},
			},
			wantErr: true,
		},
		{
			name: "valid, http submission url",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": "http://foo.bar",
				},
			},
			tlsConfig: nil,
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return circCA, nil
				},
			},
			wantErr: false,
		},
		{
			name: "valid, api.circonus.com (public cert)",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": "https://api.circonus.com",
				},
			},
			tlsConfig: nil,
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return circCA, nil
				},
			},
			wantErr: false,
		},
		{
			name: "invalid (empty) cert",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": fmt.Sprintf("https://%s:%d", brokerIP, brokerPort),
				},
			},
			tlsConfig: nil,
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return []byte(``), nil
				},
			},
			wantErr: true,
		},
		{
			name: "invalid cert",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": fmt.Sprintf("https://%s:%d", brokerIP, brokerPort),
				},
			},
			tlsConfig: nil,
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return []byte(`{"Contents":"foobar"}`), nil
				},
			},
			wantErr: true,
		},
		{
			name: "invalid broker details",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": fmt.Sprintf("https://%s:%d", brokerIP, brokerPort),
				},
			},
			tlsConfig: nil,
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return circCA, nil
				},
			},
			wantErr: true,
		},
		{
			name: "valid broker details",
			checkBundle: &apiclient.CheckBundle{
				Config: apiclient.CheckBundleConfig{
					"submission_url": fmt.Sprintf("https://%s:%d", brokerIP, brokerPort),
				},
			},
			tlsConfig: nil,
			broker: &apiclient.Broker{
				Details: []apiclient.BrokerDetail{
					{CN: "foo", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
					{CN: "bar", IP: &brokerIP, Port: &brokerPort, Status: statusActive},
				},
			},
			client: &APIMock{
				GetFunc: func(requrl string) ([]byte, error) {
					return circCA, nil
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			tc.client = tt.client
			tc.tlsConfig = tt.tlsConfig
			tc.checkBundle = tt.checkBundle
			tc.broker = tt.broker
			if tc.checkBundle != nil {
				tc.submissionURL = tt.checkBundle.Config[config.SubmissionURL]
			}

			if err := tc.setBrokerTLSConfig(); (err != nil) != tt.wantErr {
				t.Errorf("TrapCheck.setBrokerTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

var circCA = []byte(`{"contents":"# Circonus Certificate Authority G2\n-----BEGIN CERTIFICATE-----\nMIIE6zCCA9OgAwIBAgIJALY0C6uznIh+MA0GCSqGSIb3DQEBCwUAMIGpMQswCQYD\nVQQGEwJVUzERMA8GA1UECBMITWFyeWxhbmQxDzANBgNVBAcTBkZ1bHRvbjEXMBUG\nA1UEChMOQ2lyY29udXMsIEluYy4xETAPBgNVBAsTCENpcmNvbnVzMSowKAYDVQQD\nEyFDaXJjb251cyBDZXJ0aWZpY2F0ZSBBdXRob3JpdHkgRzIxHjAcBgkqhkiG9w0B\nCQEWD2NhQGNpcmNvbnVzLm5ldDAeFw0xOTEyMDYyMDAzMzdaFw0zOTEyMDYyMDAz\nMzdaMIGpMQswCQYDVQQGEwJVUzERMA8GA1UECBMITWFyeWxhbmQxDzANBgNVBAcT\nBkZ1bHRvbjEXMBUGA1UEChMOQ2lyY29udXMsIEluYy4xETAPBgNVBAsTCENpcmNv\nbnVzMSowKAYDVQQDEyFDaXJjb251cyBDZXJ0aWZpY2F0ZSBBdXRob3JpdHkgRzIx\nHjAcBgkqhkiG9w0BCQEWD2NhQGNpcmNvbnVzLm5ldDCCASIwDQYJKoZIhvcNAQEB\nBQADggEPADCCAQoCggEBAK9oN6wBfBgjRYKBbL0Hllcr9TR2e0wIDGhk15Ltym32\nzkndEcNKoz61BBJZGalPYDQ8khGQEJAHF6jE/q+qPFHA7vMoIll0frD/C8MM09PK\nwvvw+HfnRLjnAWwmefDsE+zhdXlOMnsRPPmMHOCYw0RYe4z8Zna3Jl57zZt8zlKh\nFnWRsZg8zc5dFQsAteu2vV+ZSYXUZyj2IgmqaeKgjyUL09ByBKH+weS0ICXiIS51\n8lEmofj87ceBMRJHjIwnFr9dRvj3YU/DZVL8NVy91jBHPw9PhLV8XQRh6oQXkrSr\nvlcs3NN2FNqWIfZmL6g8/OCCXr3oFgotumGUc7H/cS0CAwEAAaOCARIwggEOMB0G\nA1UdDgQWBBRk0xgZQ17grBWWZbRRTzZfqlAd4zCB3gYDVR0jBIHWMIHTgBRk0xgZ\nQ17grBWWZbRRTzZfqlAd46GBr6SBrDCBqTELMAkGA1UEBhMCVVMxETAPBgNVBAgT\nCE1hcnlsYW5kMQ8wDQYDVQQHEwZGdWx0b24xFzAVBgNVBAoTDkNpcmNvbnVzLCBJ\nbmMuMREwDwYDVQQLEwhDaXJjb251czEqMCgGA1UEAxMhQ2lyY29udXMgQ2VydGlm\naWNhdGUgQXV0aG9yaXR5IEcyMR4wHAYJKoZIhvcNAQkBFg9jYUBjaXJjb251cy5u\nZXSCCQC2NAurs5yIfjAMBgNVHRMEBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQCq\n9yqOHBWeP65jUnr+pn5nf9+dJhIQ/zgEiIygUwJoSo0+OG1fwfXEeQMQdrYJlTfT\nLLgAlK/lJ0fXfS4ruMwyOnH5/2UTrh2eE1u8xToKg7afbaIoO/sg002f3qod1MRx\nJYPppNW16wG4kaBKOXJY6LzqXeaStCFotrer5Wt4tl/xOaVav1lmdXC8V3vUtoMJ\nFasyBc3tBlgKRJ0f2ijD+P6vEie4w8gJMSurqqKskiY+2zuNzClki0bqCi06m0lt\nTESkwBQfV80GJXyz4kTQIZgGnwLcNE9GOlihWX2axTpW7RwpX25lOaMtu+vZtao/\nyQRBN07uOh4gEhJIngzr\n-----END CERTIFICATE-----\n"}`)
