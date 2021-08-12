// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/circonus-labs/go-apiclient"
	"github.com/circonus-labs/go-apiclient/config"
)

func (tc *TrapCheck) initializeCheck() error {
	cfg := tc.checkConfig
	if cfg == nil {
		cfg = &apiclient.CheckBundle{}
	}

	if cfg.CID != "" {
		return tc.fetchCheckBundle()
	}

	return tc.initCheckBundle(cfg)
}

func (tc *TrapCheck) refreshCheck() (bool, error) {
	if tc.custSubmissionURL != "" {
		return false, nil // custom submission url provided, check can't be refreshed
	}
	if tc.checkBundle == nil {
		return false, fmt.Errorf("invalid state check bundle nil")
	}

	cid := tc.checkBundle.CID
	bundle, err := tc.client.FetchCheckBundle(apiclient.CIDType(&cid))
	if err != nil {
		return false, fmt.Errorf("fetching check bundle: %w", err)
	}

	tc.checkBundle = bundle
	if surl, ok := tc.checkBundle.Config[config.SubmissionURL]; ok {
		tc.submissionURL = surl
	} else {
		return false, fmt.Errorf("no submission url found in check bundle config")
	}

	// force refresh of broker and tls config as well
	tc.tlsConfig = nil
	tc.broker = nil
	if err := tc.setBrokerTLSConfig(); err != nil {
		return false, err
	}
	return true, nil
}

func (tc *TrapCheck) initCheckBundle(cfg *apiclient.CheckBundle) error {

	if err := tc.applyCheckBundleDefaults(cfg); err != nil {
		return err
	}

	found, err := tc.findCheckBundle(cfg)
	if err != nil {
		return fmt.Errorf("searching for check bundle: %w", err)
	}

	if !found {
		if err := tc.createCheckBundle(cfg); err != nil {
			return err
		}
	}

	return nil
}

func (tc *TrapCheck) findCheckBundle(cfg *apiclient.CheckBundle) (bool, error) {
	// e.g. (active:1)(type:"httptrap:cua:host:linux")(host:"el7-cua-test")(tags:service:circonus-unified-agentd)
	searchCriteria := apiclient.SearchQueryType(
		fmt.Sprintf(`(active:1)(type:"%s")(target:"%s")(tags:%s)`,
			cfg.Type,
			cfg.Target,
			strings.Join(tc.checkSearchTags, ",")))

	bundles, err := tc.client.SearchCheckBundles(&searchCriteria, nil)
	if err != nil {
		return false, fmt.Errorf("search check bundles (%s): %w", searchCriteria, err)
	}

	numBundles := len(*bundles)
	switch {
	case numBundles == 1:
		bundle := (*bundles)[0]
		tc.checkBundle = &bundle
		return true, nil
	case numBundles > 1:
		found := 0
		idx := -1
		for i, bundle := range *bundles {
			if bundle.Type == cfg.Type {
				found++
				idx = i
			}
		}
		switch {
		case found == 0:
			return false, fmt.Errorf("multiple (%d) bundles found matching '%s' none are type (%s)", numBundles, searchCriteria, cfg.Type)
		case found == 1:
			bundle := (*bundles)[idx]
			tc.checkBundle = &bundle
			return true, nil
		case found > 1:
			return false, fmt.Errorf("multiple (%d) check bundles found matching '%s'", found, searchCriteria)
		}
	}

	return false, nil // trigger check create
}

func (tc *TrapCheck) createCheckBundle(cfg *apiclient.CheckBundle) error {
	if cfg == nil {
		return fmt.Errorf("invalid check bundle config (nil)")
	}
	// add broker here, no reason to do it in applying defaults as that's
	// done every time, even when a check could be found (so no point "selecting"
	// a broker to create a check, when a check already exists)
	if len(cfg.Brokers) == 0 {
		err := tc.getBroker(cfg.Type)
		if err != nil {
			return err
		}
		cfg.Brokers = []string{tc.broker.CID}
	}
	bundle, err := tc.client.CreateCheckBundle(cfg)
	if err != nil {
		return fmt.Errorf("create check bundle: %w", err)
	}
	tc.checkBundle = bundle
	return nil
}

func (tc *TrapCheck) fetchCheckBundle() error {
	bundle, err := tc.client.FetchCheckBundle(&tc.checkConfig.CID)
	if err != nil {
		return fmt.Errorf("retrieving check bundle (%s): %w", tc.checkConfig.CID, err)
	}

	if bundle.Status != statusActive {
		return fmt.Errorf("invalid check bundle (%s), not active", bundle.CID)
	}

	if _, found := bundle.Config[config.SubmissionURL]; !found {
		return fmt.Errorf("invalid check bundle (%s) no '%s' in config", bundle.CID, config.SubmissionURL)
	}

	tc.checkBundle = bundle

	return nil
}

func (tc *TrapCheck) applyCheckBundleDefaults(cfg *apiclient.CheckBundle) error {
	_, an := filepath.Split(os.Args[0])
	hn, err := os.Hostname()
	if err != nil {
		hn = "unknown"
	}

	// check type
	if cfg.Type == "" {
		cfg.Type = "httptrap"
	}

	// force status to active
	if cfg.Status == "" {
		cfg.Status = statusActive
	}

	// must be set to an empty array...
	cfg.Metrics = []apiclient.CheckBundleMetric{}

	// metric filters
	if len(cfg.MetricFilters) == 0 {
		// cfg.MetricFilters = [][]string{{"deny", "^$", ""}, {"allow", "^.+$", ""}}
		cfg.MetricFilters = [][]string{{"allow", ".", ""}}
	}

	// search tag, and check tags
	if len(tc.checkSearchTags) == 0 {
		tc.checkSearchTags = apiclient.TagType{"service:" + an}
	}
	// NOTE: not needed, UI/API provide different results - see search above
	// if strings.Count(cfg.Type, ":") > 0 {
	// 	if !strings.Contains(strings.Join(tc.checkSearchTag, ","), "ext_type:") {
	// 		tc.checkSearchTag = append(tc.checkSearchTag, "ext_type:"+cfg.Type)
	// 	}
	// }
	if len(cfg.Tags) == 0 {
		cfg.Tags = tc.checkSearchTags
	} else {
		cfg.Tags = append(cfg.Tags, tc.checkSearchTags...)
	}

	// display name, target, notes
	instanceID := fmt.Sprintf("%s:%s", hn, an)
	if cfg.DisplayName == "" {
		cfg.DisplayName = instanceID
	}
	if cfg.Target == "" {
		cfg.Target = instanceID
	}
	if cfg.Notes == nil {
		notes := "tcid:" + instanceID
		cfg.Notes = &notes
	}

	// period & timeout
	if cfg.Period == 0 {
		cfg.Period = 60
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10
	}

	// config options (specific to httptrap)
	if len(cfg.Config) == 0 {
		cfg.Config = make(map[config.Key]string)
	}

	// async metrics, enabled
	if val, ok := cfg.Config[config.AsyncMetrics]; !ok || val == "" {
		cfg.Config[config.AsyncMetrics] = "true"
	}

	// submission url secret
	if val, ok := cfg.Config[config.Secret]; !ok || val == "" {
		secret, err := makeSecret()
		if err != nil {
			secret = "myS3cr3t"
		}
		cfg.Config[config.Secret] = secret
	}

	return nil
}

// Create a dynamic secret to use with a new check.
func makeSecret() (string, error) {
	hash := sha256.New()
	x := make([]byte, 2048)
	if _, err := rand.Read(x); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	if _, err := hash.Write(x); err != nil {
		return "", fmt.Errorf("hash write: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil))[0:16], nil
}
