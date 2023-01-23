// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/circonus-labs/go-apiclient"
	"github.com/circonus-labs/go-apiclient/config"
)

const (
	statusActive                 = "active"
	enterpriseType               = "enterprise"
	circonusType                 = "circonus"
	defaultBrokerMaxResponseTime = "500ms" // 500 milliseconds
)

func (tc *TrapCheck) fetchBroker(cid, checkType string) error {
	if cid == "" {
		return fmt.Errorf("invalid broker cid (empty)")
	}
	if checkType == "" {
		return fmt.Errorf("invalid check type (empty)")
	}
	broker, err := brokerList.GetBroker(cid)
	// broker, err := tc.client.FetchBroker(apiclient.CIDType(&cid))
	if err != nil {
		return fmt.Errorf("retrieving broker (%s): %w", cid, err)
	}
	if valid, err := tc.isValidBroker(&broker, checkType); !valid {
		return fmt.Errorf("%s (%s) is an invalid broker for check type %s: %w", tc.broker.Name, tc.checkConfig.Brokers[0], checkType, err)
	}
	tc.broker = &broker
	return nil
}

func (tc *TrapCheck) getBroker(checkType string) error {
	//
	// caller defiened specific broker, try to use it
	//
	if tc.checkConfig != nil && len(tc.checkConfig.Brokers) > 0 {
		return tc.fetchBroker(tc.checkConfig.Brokers[0], checkType)
	}

	//
	// otherwise, select an applicable broker
	//
	var list *[]apiclient.Broker

	if len(tc.brokerSelectTags) > 0 {
		// filter := apiclient.SearchFilterType{
		// 	"f__tags_has": tc.brokerSelectTags,
		// }
		bl, err := brokerList.SearchBrokerList(tc.brokerSelectTags) //tc.client.SearchBrokers(nil, &filter)
		if err != nil {
			return fmt.Errorf("search brokers: %w", err)
		}
		list = bl
	} else {
		bl, err := brokerList.GetBrokerList() // tc.client.FetchBrokers()
		if err != nil {
			return fmt.Errorf("fetch brokers: %w", err)
		}
		list = bl
	}

	if len(*list) == 0 {
		return fmt.Errorf("zero brokers found")
	}

	validBrokers := make(map[string]apiclient.Broker)
	haveEnterprise := false

	for _, broker := range *list {
		broker := broker
		valid, err := tc.isValidBroker(&broker, checkType)
		if err != nil {
			tc.Log.Debugf("skipping, broker '%s' -- invalid: %s", broker.Name, err)
			continue
		}
		if !valid {
			tc.Log.Debugf("skipping, broker '%s' -- invalid", broker.Name)
			continue
		}
		validBrokers[broker.CID] = broker
		if broker.Type == enterpriseType {
			haveEnterprise = true
		}
	}

	if haveEnterprise { // eliminate non-enterprise brokers from valid brokers
		for k, v := range validBrokers {
			if v.Type != enterpriseType {
				delete(validBrokers, k)
			}
		}
	}

	if len(validBrokers) == 0 {
		return fmt.Errorf("found %d broker(s), zero are valid", len(*list))
	}

	validBrokerKeys := reflect.ValueOf(validBrokers).MapKeys()
	maxBrokers := big.NewInt(int64(len(validBrokerKeys)))
	bidx, err := rand.Int(rand.Reader, maxBrokers)
	if err != nil {
		return fmt.Errorf("rand: %w", err)
	}
	selectedBroker := validBrokers[validBrokerKeys[bidx.Uint64()].String()]

	tc.Log.Infof("selected broker '%s'", selectedBroker.Name)
	tc.broker = &selectedBroker

	return nil
}

func (tc *TrapCheck) isValidBroker(broker *apiclient.Broker, checkType string) (bool, error) {
	if broker == nil {
		return false, fmt.Errorf("invalid state, broker (nil)")
	}

	var brokerHost string
	var brokerPort string

	if broker.Type != circonusType && broker.Type != enterpriseType {
		return false, fmt.Errorf("broker '%s' has unknown type (%s)", broker.Name, broker.Type)
	}

	if len(broker.Details) == 0 {
		return false, fmt.Errorf("broker '%s' invalid, no instance details", broker.Name)
	}

	httpProxy := os.Getenv("HTTP_PROXY")
	httpsProxy := os.Getenv("HTTPS_PROXY")

	for _, detail := range broker.Details {
		detail := detail

		// broker must be active
		if detail.Status != statusActive {
			tc.Log.Debugf("skipping -- broker '%s' instance '%s' -- not active (%s)", broker.Name, detail.CN, detail.Status)
			continue
		}

		// broker must have module loaded for the check type to be used
		if ok, err := tc.brokerSupportsCheckType(checkType, &detail); !ok {
			tc.Log.Debugf("skipping -- broker '%s' instance '%s' -- does not support check type (%s): %s", broker.Name, detail.CN, checkType, err)
			continue
		}

		if detail.ExternalPort != 0 {
			brokerPort = strconv.Itoa(int(detail.ExternalPort))
		} else {
			if detail.Port != nil && *detail.Port != 0 {
				brokerPort = strconv.Itoa(int(*detail.Port))
			} else {
				brokerPort = "43191"
			}
		}

		if detail.ExternalHost != nil && *detail.ExternalHost != "" {
			brokerHost = *detail.ExternalHost
		} else if detail.IP != nil && *detail.IP != "" {
			brokerHost = *detail.IP
		}

		if brokerHost == "" {
			tc.Log.Debugf("skipping -- broker '%s' instance '%s' -- no IP or external host set", broker.Name, detail.CN)
			continue
		}

		if brokerHost == "trap.noit.circonus.net" && brokerPort != "443" {
			brokerPort = "443"
		}
		if brokerHost == "api.circonus.net" && brokerPort != "443" {
			brokerPort = "443"
		}

		// do not direct connect to test broker, if a proxy env var is set and check is httptrap
		if strings.Contains(strings.ToLower(checkType), "httptrap") {
			if httpProxy != "" || httpsProxy != "" {
				tc.Log.Debugf("skipping connection test, proxy environment var(s) set -- HTTP:'%s' HTTPS:'%s'", httpProxy, httpsProxy)
				return true, nil
			}
		}

		retries := 5
		target := fmt.Sprintf("%s:%s", brokerHost, brokerPort)
		for attempt := 1; attempt <= retries; attempt++ {
			// broker must be reachable and respond within designated time
			conn, err := net.DialTimeout("tcp", target, tc.brokerMaxResponseTime)
			if err == nil {
				conn.Close()
				tc.Log.Debugf("broker '%s' instance '%s' -- is valid", broker.Name, detail.CN)
				return true, nil
			}

			tc.Log.Debugf("broker '%s' instance '%s' -- unable to connect (%s): %v -- retry in 2s, attempt %d of %d", broker.Name, detail.CN, target, err, attempt, retries)
			time.Sleep(2 * time.Second)
		}
	}

	return false, fmt.Errorf("no valid broker instances found")
}

// Verify broker supports the check type to be used.
func (tc *TrapCheck) brokerSupportsCheckType(checkType string, details *apiclient.BrokerDetail) (bool, error) {
	if details == nil {
		return false, fmt.Errorf("invalid broker details (nil)")
	}

	if checkType == "" {
		return false, fmt.Errorf("invalid check type (empty)")
	}

	baseType := checkType

	if idx := strings.Index(baseType, ":"); idx > 0 {
		baseType = baseType[0:idx]
	}

	for _, module := range details.Modules {
		if module == baseType {
			return true, nil
		}
	}

	return false, fmt.Errorf("check type '%s' not found in broker modules (%s)", baseType, strings.Join(details.Modules, ","))

}

func (tc *TrapCheck) getBrokerCNList() (string, string, error) {
	if tc.broker == nil {
		return "", "", fmt.Errorf("invalid state, broker is nil")
	}
	if tc.checkBundle == nil {
		return "", "", fmt.Errorf("invalid state, check bundle is nil")
	}
	submissionURL := tc.checkBundle.Config[config.SubmissionURL]
	u, err := url.Parse(submissionURL)
	if err != nil {
		return "", "", fmt.Errorf("parse submission URL: %w", err)
	}

	hostParts := strings.Split(u.Host, ":")
	host := hostParts[0]

	if net.ParseIP(host) == nil { // it's an FQDN (or at the very least, not an ip)
		return u.Hostname(), "", nil
	}

	cn := ""
	cnList := make([]string, 0, len(tc.broker.Details))
	for _, detail := range tc.broker.Details {
		if detail.Status != statusActive {
			continue
		}
		if detail.IP != nil && *detail.IP == host {
			if cn == "" {
				cn = detail.CN
			}
			cnList = append(cnList, detail.CN)
		} else if detail.ExternalHost != nil && *detail.ExternalHost == host {
			if cn == "" {
				cn = detail.CN
			}
			cnList = append(cnList, detail.CN)
		}
	}

	if len(cnList) == 0 {
		return "", "", fmt.Errorf("unable to match URL host (%s) to broker instance", u.Host)
	}

	return cn, strings.Join(cnList, ","), nil
}
