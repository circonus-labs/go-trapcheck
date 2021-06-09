// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/circonus-labs/go-apiclient"
	"github.com/circonus-labs/go-apiclient/config"
)

type Config struct {
	// Client is a valid circonus go-apiclient instance
	Client API
	// CheckConfig is a valid circonus go-apiclient.CheckBundle configuration
	// or nil for defaults
	CheckConfig *apiclient.CheckBundle
	// SubmitTLSConfig is a *tls.Config to use when submitting to the broker
	SubmitTLSConfig *tls.Config
	// Logger interface for logging
	Logger Logger
	// SubmissionURL explicit submission url (e.g. submitting to an agent, if tls used a SubmitTLSConfig is required)
	SubmissionURL string
	// BrokerMaxResponseTime defines the timeout in which brokers must respond when selecting
	BrokerMaxResponseTime string
	// TraceMetrics path to write traced metrics to (must be writable by the user running app)
	TraceMetrics string
	// BrokerSelectTags defines a tag to use when selecting a broker to use (when creating a check)
	BrokerSelectTags apiclient.TagType
	// CheckSearchTags defines a tag to use when searching for a check
	CheckSearchTags apiclient.TagType
}

type TrapCheck struct {
	client                API
	Log                   Logger
	checkConfig           *apiclient.CheckBundle
	checkBundle           *apiclient.CheckBundle
	broker                *apiclient.Broker
	tlsConfig             *tls.Config
	custTLSConfig         *tls.Config
	submissionURL         string
	custSubmissionURL     string
	traceMetrics          string
	checkSearchTags       apiclient.TagType
	brokerSelectTags      apiclient.TagType
	brokerMaxResponseTime time.Duration
}

// New creates a new TrapCheck instance
// it will create a check if it is not able to find
// one based on the passed Check Config and Check Search Tag.
func New(cfg *Config) (*TrapCheck, error) {
	if cfg == nil {
		return nil, fmt.Errorf("invalid configuration  (nil)")
	}

	if cfg.Client == nil {
		return nil, fmt.Errorf("invalid configuration (nil api client)")
	}

	tc := &TrapCheck{
		client:            cfg.Client,
		checkConfig:       cfg.CheckConfig,
		checkSearchTags:   cfg.CheckSearchTags,
		custSubmissionURL: cfg.SubmissionURL,
		custTLSConfig:     cfg.SubmitTLSConfig,
		brokerSelectTags:  cfg.BrokerSelectTags,
		checkBundle:       nil,
		broker:            nil,
		tlsConfig:         nil,
		submissionURL:     "",
	}

	if cfg.Logger != nil {
		tc.Log = cfg.Logger
	} else {
		tc.Log = &LogWrapper{
			Log:   log.New(ioutil.Discard, "", log.LstdFlags),
			Debug: false,
		}
	}

	dur := cfg.BrokerMaxResponseTime
	if dur == "" {
		dur = defaultBrokerMaxResponseTime
	}
	maxDur, err := time.ParseDuration(dur)
	if err != nil {
		return nil, fmt.Errorf("parsing broker max response time (%s): %w", dur, err)
	}
	tc.brokerMaxResponseTime = maxDur

	if cfg.TraceMetrics != "" {
		err := testTraceMetricsDir(cfg.TraceMetrics)
		if err != nil {
			tc.Log.Warnf("trace metrics directory (%s): %s -- disabling", cfg.TraceMetrics, err)
		} else {
			tc.traceMetrics = cfg.TraceMetrics
		}
	}

	if cfg.CheckConfig != nil {
		// verify that if the check type is set, it is a variant of httptrap
		// this module ONLY deals with httptraps.
		if cfg.CheckConfig.Type != "" && !strings.HasPrefix(cfg.CheckConfig.Type, "httptrap") {
			return nil, fmt.Errorf("check type must be httptrap variant (%s)", cfg.CheckConfig.Type)
		}
	}

	tc.submissionURL = tc.custSubmissionURL
	if tc.submissionURL == "" {
		if err := tc.initializeCheck(); err != nil {
			return nil, err
		}
		if surl, ok := tc.checkBundle.Config[config.SubmissionURL]; ok {
			tc.submissionURL = surl
		} else {
			return nil, fmt.Errorf("no submission url found in check bundle config")
		}
	}

	if err := tc.setBrokerTLSConfig(); err != nil {
		return nil, err
	}

	return tc, nil
}

// SendMetrics submits the metrics to the broker
// metrics must be valid JSON encoded data for the broker httptrap check
// returns trap results in a structure or an error.
func (tc *TrapCheck) SendMetrics(ctx context.Context, metrics *strings.Builder) (*TrapResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	result, refresh, submitErr := tc.submit(ctx, metrics)

	if refresh {
		// try to refresh the check and reset the tls config
		// check moved to a different broker, etc.
		refreshed, refreshErr := tc.refreshCheck()
		if refreshErr != nil {
			return nil, refreshErr
		}
		if !refreshed {
			// if no refresh error, but it couldn't be refreshed (e.g. custom
			// submission url) just return the original submit error
			return nil, fmt.Errorf("unable to refresh: %w", submitErr)
		}
		// try submission again, if it fails again just pass the error back to the caller
		result, _, submitErr = tc.submit(ctx, metrics)
	}

	return result, submitErr
}

// GetCheckBundle returns the trap check bundle currently in use - can be used
// for caching checks on disk and re-using the ckeck quickly by passing
// the CID in via the check bundle config.
func (tc *TrapCheck) GetCheckBundle() (*apiclient.CheckBundle, error) {
	if tc.checkBundle == nil {
		return nil, fmt.Errorf("trap check not initialized/created")
	}
	return tc.checkBundle, nil
}

// GetBrokerTLSConfig returns the current tls config - can be used
// for pre-seeding multiple check creation without repeatedly
// calling the API for the same CA cert - returns tls config, error.
func (tc *TrapCheck) GetBrokerTLSConfig() (*tls.Config, error) {
	if public, err := tc.isPublicBroker(); err != nil {
		return nil, err
	} else if public {
		return nil, nil
	}
	if tc.tlsConfig == nil {
		return nil, fmt.Errorf("tls config has not been initialized")
	}
	return tc.tlsConfig.Clone(), nil
}

func (tc *TrapCheck) isPublicBroker() (bool, error) {
	if tc.checkBundle == nil {
		return false, fmt.Errorf("invalid state, check bundle not initialized")
	}
	if tc.submissionURL == "" {
		return false, fmt.Errorf("invalid state, no submission url")
	}
	return strings.Contains(tc.submissionURL, "api.circonus.com"), nil
}

// TraceMetrics allows changing the tracing of metric submissions dynamically,
// pass "" to disable tracing going forward. returns current setting or error.
// on error, the current setting will not be changed.
// Note: if going from no Logger to trace="-" the Logger will need to be set.
func (tc *TrapCheck) TraceMetrics(trace string) (string, error) {
	curr := tc.traceMetrics
	if trace != "" {
		err := testTraceMetricsDir(trace)
		if err != nil {
			return curr, fmt.Errorf("trace metrics (%s): %w", trace, err)
		}
	}
	tc.traceMetrics = trace
	return curr, nil
}

// testTraceMetricsDir verifies the trace metrics directory exists and is writeable.
func testTraceMetricsDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("invalid trace setting (empty)")
	}
	// will go to logger.Infof
	if dir == "-" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("unable to stat (%s): %w", dir, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("not a directory (%s)", dir)
	}

	tf, err := ioutil.TempFile(dir, "wtest")
	if err != nil {
		return fmt.Errorf("unable to write to (%s): %w", dir, err)
	}

	defer os.Remove(tf.Name())

	return nil
}
