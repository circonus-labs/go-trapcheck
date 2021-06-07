// Copyright (c) 2021 Circonus, Inc. <support@circonus.com>
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//

package trapcheck

//go:generate moq -out api_moq_test.go . API

import "github.com/circonus-labs/go-apiclient"

type API interface {
	// generic methods
	Get(requrl string) ([]byte, error)
	// broker methods
	FetchBroker(cid apiclient.CIDType) (*apiclient.Broker, error)
	FetchBrokers() (*[]apiclient.Broker, error)
	SearchBrokers(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.Broker, error)
	// check bundle methods
	FetchCheckBundle(cid apiclient.CIDType) (*apiclient.CheckBundle, error)
	CreateCheckBundle(cfg *apiclient.CheckBundle) (*apiclient.CheckBundle, error)
	SearchCheckBundles(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.CheckBundle, error)
}
