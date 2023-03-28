package brokerlist

//go:generate moq -out api_moq_test.go . API

import "github.com/circonus-labs/go-apiclient"

type API interface {
	// broker methods
	FetchBroker(cid apiclient.CIDType) (*apiclient.Broker, error)
	FetchBrokers() (*[]apiclient.Broker, error)
	SearchBrokers(searchCriteria *apiclient.SearchQueryType, filterCriteria *apiclient.SearchFilterType) (*[]apiclient.Broker, error)
}
