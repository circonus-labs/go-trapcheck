package brokerlist

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/circonus-labs/go-apiclient"
)

// var once sync.Once

type BrokerList interface {
	RefreshBrokers() error
	FetchBrokers() error
	GetBrokerList() (*[]apiclient.Broker, error)
	GetBroker(cid string) (apiclient.Broker, error)
	SearchBrokerList(searchTags apiclient.TagType) (*[]apiclient.Broker, error)
	SetClient(API) error
}

type brokerList struct {
	lastRefresh time.Time
	logger      Logger
	client      API
	brokers     *[]apiclient.Broker
	sync.Mutex
}

var brokerListInstance *brokerList

func Init(client API, logger Logger) error {
	if client == nil {
		return fmt.Errorf("invalid init call, client is nil")
	}

	if logger == nil {
		return fmt.Errorf("invalid init call, logger is nil")
	}

	if brokerListInstance != nil {
		return nil
	}

	brokerListInstance = &brokerList{
		client: client,
		logger: logger,
	}
	return brokerListInstance.FetchBrokers()
}

func GetInstance() (BrokerList, error) { //nolint:revive
	if brokerListInstance == nil {
		return nil, fmt.Errorf("broker list not initialized")
	}
	return brokerListInstance, nil
}

func (bl *brokerList) SetClient(client API) error {
	if client == nil {
		return fmt.Errorf("invalid init call, client is nil")
	}

	bl.client = client

	return nil
}

func (bl *brokerList) RefreshBrokers() error {
	// only refresh if it's been at least five minutes since last refresh
	// to prevent API request storms.
	if time.Since(bl.lastRefresh) > 5*time.Minute {
		return bl.FetchBrokers()
	}
	return nil
}

func (bl *brokerList) FetchBrokers() error {
	bl.Lock()
	defer bl.Unlock()

	bl.logger.Infof("fetching broker list")
	list, err := bl.client.FetchBrokers()
	if err != nil {
		return fmt.Errorf("error fetching broker list: %w", err)
	}

	bl.brokers = list

	return nil
}

func (bl *brokerList) GetBrokerList() (*[]apiclient.Broker, error) {
	bl.Lock()
	defer bl.Unlock()

	if bl.brokers == nil {
		return nil, fmt.Errorf("invalid state, broker list is nil")
	}

	if len(*bl.brokers) == 0 {
		return nil, fmt.Errorf("invalid state, empty broker list")
	}

	list := *bl.brokers

	return &list, nil
}

func (bl *brokerList) GetBroker(cid string) (apiclient.Broker, error) {
	if cid == "" {
		return apiclient.Broker{}, fmt.Errorf("invalid cid (empty)")
	}

	bl.Lock()
	defer bl.Unlock()

	if bl.brokers == nil {
		return apiclient.Broker{}, fmt.Errorf("invalid state, broker list is nil")
	}

	if len(*bl.brokers) == 0 {
		if err := bl.FetchBrokers(); err != nil {
			return apiclient.Broker{}, fmt.Errorf("invalid state, broker list len is 0, unable to fetch broker list: %w", err)
		}
		if len(*bl.brokers) == 0 {
			return apiclient.Broker{}, fmt.Errorf("invalid state, no brokers in list")
		}
	}

	for _, b := range *bl.brokers {
		if b.CID == cid {
			bl.logger.Infof("using cached broker %s", b.CID)
			return b, nil
		}
	}

	return apiclient.Broker{}, fmt.Errorf("no broker with CID (%s) found", cid)
}

func (bl *brokerList) SearchBrokerList(searchTags apiclient.TagType) (*[]apiclient.Broker, error) {
	bl.Lock()
	defer bl.Unlock()

	if bl.brokers == nil {
		return nil, fmt.Errorf("invalid state, broker list is nil")
	}

	if len(*bl.brokers) == 0 {
		return nil, fmt.Errorf("invalid state, empty broker list")
	}

	var list []apiclient.Broker

	for _, b := range *bl.brokers {
		numTagsFound := 0
		for _, t := range b.Tags {
			for _, st := range searchTags {
				if strings.EqualFold(t, st) {
					numTagsFound++
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
