// Copyright (c) 2023 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/circonus-labs/go-apiclient"
)

type brokers struct {
	sync.Mutex
	lastRefresh time.Time
	client      API
	logger      Logger
	brokerList  *[]apiclient.Broker
}

var brokerList *brokers

func init() {
	brokerList = &brokers{}
}

func (bl *brokers) Init(client API, logger Logger) error {
	if client == nil {
		return fmt.Errorf("invalid init call, client is nil")
	}

	if logger == nil {
		return fmt.Errorf("invalid init call, logger is nil")
	}

	bl.Lock()

	if bl.client == nil {
		bl.client = client
	}

	if bl.logger == nil {
		bl.logger = logger
	}

	if bl.client != nil && bl.brokerList == nil {
		bl.Unlock()
		return bl.FetchBrokers()
	}

	bl.Unlock()
	return nil
}

func (bl *brokers) RefreshBrokers() error {
	// only refresh if it's beein 5 minutes since last refresh
	// to prevent API request storms.
	if time.Since(bl.lastRefresh) > 5*time.Minute {
		return bl.FetchBrokers()
	}
	return nil
}

func (bl *brokers) FetchBrokers() error {
	bl.Lock()
	defer bl.Unlock()

	list, err := bl.client.FetchBrokers()
	if err != nil {
		return fmt.Errorf("error fetching broker list: %w", err)
	}

	bl.brokerList = list

	return nil
}

func (bl *brokers) GetBrokerList() (*[]apiclient.Broker, error) {
	bl.Lock()
	defer bl.Unlock()

	if bl.brokerList == nil {
		return nil, fmt.Errorf("invalid state, broker list is nil")
	}

	if len(*bl.brokerList) == 0 {
		return nil, fmt.Errorf("invalid state, empty broker list")
	}

	list := *bl.brokerList

	return &list, nil
}

func (bl *brokers) GetBroker(cid string) (apiclient.Broker, error) {
	if cid == "" {
		return apiclient.Broker{}, fmt.Errorf("invalid cid (empty)")
	}

	bl.Lock()
	defer bl.Unlock()

	if len(*bl.brokerList) == 0 {
		if err := bl.FetchBrokers(); err != nil {
			return apiclient.Broker{}, fmt.Errorf("invalid state, broker list len is 0, unable to fetch broker list: %w", err)
		}
		if len(*bl.brokerList) == 0 {
			return apiclient.Broker{}, fmt.Errorf("invalid state, no brokers in list")
		}
	}

	for _, b := range *bl.brokerList {
		if b.CID == cid {
			return b, nil
		}
	}

	return apiclient.Broker{}, fmt.Errorf("no broker with CID (%s) found", cid)
}

func (bl *brokers) SearchBrokerList(searchTags apiclient.TagType) (*[]apiclient.Broker, error) {
	bl.Lock()
	bl.Unlock()

	if bl.brokerList == nil {
		return nil, fmt.Errorf("invalid state, broker list is nil")
	}

	if len(*bl.brokerList) == 0 {
		return nil, fmt.Errorf("invalid state, empty broker list")
	}

	var list []apiclient.Broker

	for _, b := range *bl.brokerList {
		numTagsFound := 0
		for _, t := range b.Tags {
			for _, st := range searchTags {
				if strings.EqualFold(t, st) {
					numTagsFound += 1
					break
				}
			}
		}
		if numTagsFound == len(searchTags) {
			list = append(list, b)
		}
	}

	return &list, nil
}
