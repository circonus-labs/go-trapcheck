// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/circonus-labs/go-trapcheck/internal/release"
	"github.com/google/uuid"
	"github.com/hashicorp/go-retryablehttp"
)

type TrapResult struct {
	CheckUUID       string
	Error           string `json:"error,omitempty"`
	SubmitUUID      string
	Filtered        uint64 `json:"filtered,omitempty"`
	Stats           uint64 `json:"stats"`
	SubmitDuration  time.Duration
	LastReqDuration time.Duration
	BytesSent       int
}

const (
	compressionThreshold = 1024
	traceTSFormat        = "20060102_150405.000000000"
)

func (tc *TrapCheck) submit(ctx context.Context, metricBuffer *strings.Builder) (*TrapResult, bool, error) {

	metrics := []byte(metricBuffer.String())

	if len(metrics) == 0 {
		return nil, false, fmt.Errorf("zero length data, no metrics to submit")
	}

	start := time.Now()

	if err := tc.setBrokerTLSConfig(); err != nil {
		return nil, false, fmt.Errorf("unable to set TLS config: %w", err)
	}

	var client *http.Client

	if tc.tlsConfig != nil {
		client = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:       10 * time.Second,
					KeepAlive:     3 * time.Second,
					FallbackDelay: -1 * time.Millisecond,
				}).DialContext,
				TLSClientConfig:     tc.tlsConfig,
				TLSHandshakeTimeout: 10 * time.Second,
				DisableKeepAlives:   true,
				DisableCompression:  false,
				MaxIdleConns:        1,
				MaxIdleConnsPerHost: 0,
			},
		}
	} else {
		client = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:       10 * time.Second,
					KeepAlive:     3 * time.Second,
					FallbackDelay: -1 * time.Millisecond,
				}).DialContext,
				DisableKeepAlives:   true,
				DisableCompression:  false,
				MaxIdleConns:        1,
				MaxIdleConnsPerHost: 0,
			},
		}
	}

	submitUUID := "n/a"

	payloadIsCompressed := false
	var subData *bytes.Buffer
	if len(metrics) > compressionThreshold {
		subData = bytes.NewBuffer([]byte{})
		zw := gzip.NewWriter(subData)
		n, e1 := zw.Write(metrics)
		if e1 != nil {
			return nil, false, fmt.Errorf("compressing metrics: %w", e1)
		}
		if n != len(metrics) {
			return nil, false, fmt.Errorf("write length mismatch data length %d != written length %d", len(metrics), n)
		}
		if e2 := zw.Close(); e2 != nil {
			return nil, false, fmt.Errorf("closing gzip writer: %w", e2)
		}
		payloadIsCompressed = true
	} else {
		subData = bytes.NewBuffer(metrics)
	}

	if traceDir := tc.traceMetrics; traceDir != "" {
		if traceDir == "-" {
			tc.Log.Infof("metric payload: %s", string(metrics))
		} else {
			sid, err := uuid.NewRandom()
			if err != nil {
				return nil, false, fmt.Errorf("creating new submit ID: %w", err)
			}
			submitUUID = sid.String()

			fn := path.Join(traceDir, time.Now().UTC().Format(traceTSFormat)+"_"+submitUUID+".json")
			if payloadIsCompressed {
				fn += ".gz"
			}

			if fh, e1 := os.Create(fn); e1 != nil {
				tc.Log.Errorf("creating (%s): %s -- skipping submit trace", fn, err)
			} else {
				if _, e2 := fh.Write(subData.Bytes()); e2 != nil {
					tc.Log.Errorf("writing metric trace: %s", e2)
				}
				if e3 := fh.Close(); e3 != nil {
					tc.Log.Warnf("closing metric trace (%s): %s", fn, e3)
				}
			}
		}
	}

	dataLen := subData.Len()

	var reqStart time.Time
	req, err := retryablehttp.NewRequest("PUT", tc.submissionURL, subData)
	if err != nil {
		return nil, false, fmt.Errorf("creating request: %w", err)
	}
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", release.NAME+"/"+release.VERSION)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Connection", "close")
	req.Header.Set("Content-Length", strconv.Itoa(dataLen))
	if payloadIsCompressed {
		req.Header.Set("Content-Encoding", "gzip")
	}

	retryClient := retryablehttp.NewClient()
	retryClient.HTTPClient = client
	retryClient.Logger = tc.Log // submitLogshim{logh: tc.Log.Logger()}
	retryClient.RetryWaitMin = 50 * time.Millisecond
	retryClient.RetryWaitMax = 2 * time.Second
	retryClient.RetryMax = 7
	retryClient.RequestLogHook = func(l retryablehttp.Logger, r *http.Request, attempt int) {
		if attempt > 0 {
			reqStart = time.Now()
			l.Printf("retrying... %s %d", r.URL.String(), attempt)
		}
	}

	retryClient.ResponseLogHook = func(l retryablehttp.Logger, r *http.Response) {
		if r.StatusCode != http.StatusOK {
			l.Printf("non-200 response %s: %s", r.Request.URL.String(), r.Status)
			if r.StatusCode == http.StatusNotAcceptable {
				l.Printf("broker couldn't parse payload: '%s'", string(metrics))
			}
		}
	}

	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		retry, err := retryablehttp.ErrorPropagatedRetryPolicy(ctx, resp, err)
		if retry && err != nil {
			var cie *x509.CertificateInvalidError
			if errors.As(err, &cie) {
				if cie.Reason == x509.NameMismatch {
					tc.Log.Warnf("certificate name mismatch (refreshing TLS config) common cause, new broker added to cluster or check moved to new broker: %s", cie.Detail)
					tc.clearTLSConfig()
					return false, err
				}
			} else {
				tc.Log.Warnf("request error (%s): %s", resp.Request.URL, err)
			}
		}
		return retry, nil
	}

	defer retryClient.HTTPClient.CloseIdleConnections()

	reqStart = time.Now()
	resp, err := retryClient.Do(req)
	// TODO: catch invalid cert error and return refresh check=true
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, false, fmt.Errorf("making request: %w", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound && tc.custSubmissionURL == "" {
		return nil, true, fmt.Errorf("submitting metrics (%s %s), refreshing check", req.URL.String(), resp.Status)
	} else if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("submitting metrics (%s %s)", req.URL.String(), resp.Status)
	}
	var result TrapResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, false, fmt.Errorf("parsing response (%s): %w", string(body), err)
	}

	result.CheckUUID = tc.checkBundle.CheckUUIDs[0]
	result.SubmitUUID = submitUUID
	result.SubmitDuration = time.Since(start)
	result.LastReqDuration = time.Since(reqStart)
	result.BytesSent = dataLen
	if result.Error == "" {
		result.Error = "none"
	}

	return &result, false, nil
}
