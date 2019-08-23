package spot

import (
	"sync"
	"time"

	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/aws/api"
	"k8s.io/klog"
)

type AsgAvailabilityChecker interface {
	AsgAvailability(name, iamInstanceProfile, instanceType string) bool
}

func NewSpotAvailabilityMonitor(requestLister api.AwsEC2SpotRequestLister, checkInterval, exclusionPeriod time.Duration) *spotAvailabilityMonitor {
	return &spotAvailabilityMonitor{
		requestService:  api.NewEC2SpotRequestService(requestLister),
		exclusionPeriod: exclusionPeriod,
		checkInterval:   checkInterval,
		mux:             sync.RWMutex{},
		requestCache: &spotRequestCache{
			createTime: time.Now(),
			cache:      make([]*api.SpotRequest, 0),
			mux:        sync.RWMutex{},
		},
		statusCache: &asgStatusCache{
			asgNames: make([]api.AWSAsgName, 0),
			cache:    make(map[api.AWSAsgName]*asgSpotStatus, 0),
			mux:      sync.RWMutex{},
		},
	}
}

type asgSpotStatus struct {
	AsgName            api.AWSAsgName
	IamInstanceProfile api.AWSIamInstanceProfile
	InstanceType       api.AWSInstanceType
	Available          bool
	statusChangeTime   time.Time
}

type spotAvailabilityMonitor struct {
	requestService  api.SpotRequestService
	checkInterval   time.Duration
	mux             sync.RWMutex
	requestCache    *spotRequestCache
	statusCache     *asgStatusCache
	exclusionPeriod time.Duration
}

func (m *spotAvailabilityMonitor) Run() {
	klog.V(3).Info("spot availability monitoring started")
	// monitor ad infinitum.
	for {
		select {
		case <-time.After(m.checkInterval):
			{
				err := m.roundtrip()
				if err != nil {
					klog.Errorf("spot availability check roundtrip failed: %v", err)
				} else {
					klog.V(3).Info("successful spot availability check roundtrip")
				}
			}
		}
	}
}

func (m *spotAvailabilityMonitor) roundtrip() error {
	err := m.updateRequestCache()
	if err != nil {
		return err
	}

	asgNames := m.statusCache.asgNameList()

	for _, asgName := range asgNames {
		asgStatus := m.statusCache.get(asgName)
		status := m.requestStatus(*asgStatus)

		if asgStatus.Available != status {
			if status == true {
				if time.Now().Sub(asgStatus.statusChangeTime) < m.exclusionPeriod {
					// an ASG remains unavailable for a fixed period of time
					continue
				}
			}

			m.statusCache.update(asgName, status)
		}
	}

	return nil
}

func (m *spotAvailabilityMonitor) updateRequestCache() error {
	spotRequests, err := m.requestService.List()
	if err != nil {
		return err
	}

	m.requestCache.refresh(spotRequests)

	return nil
}

func (m *spotAvailabilityMonitor) AsgAvailability(name, iamInstanceProfile, instanceType string) bool {
	asgStatus := m.asgStatus(name, iamInstanceProfile, instanceType)
	return asgStatus.Available
}

func (m *spotAvailabilityMonitor) asgStatus(name, iamInstanceProfile, instanceType string) asgSpotStatus {
	castedName := api.AWSAsgName(name)

	var asgStatus *asgSpotStatus

	exists := m.statusCache.exists(castedName)

	if !exists {
		asgStatus = &asgSpotStatus{
			AsgName:            castedName,
			IamInstanceProfile: api.AWSIamInstanceProfile(iamInstanceProfile),
			InstanceType:       api.AWSInstanceType(instanceType),
			Available:          true,
			statusChangeTime:   time.Time{},
		}

		asgStatus.Available = m.requestStatus(*asgStatus)

		m.statusCache.add(castedName, asgStatus)
	} else {
		asgStatus = m.statusCache.get(castedName)
	}

	return *asgStatus
}

func (m *spotAvailabilityMonitor) requestStatus(status asgSpotStatus) bool {
	asgRequests := m.requestCache.findRequests(status.IamInstanceProfile, status.InstanceType)

	if len(asgRequests) > 0 {
		for _, request := range asgRequests {
			if request.State == api.AWSSpotRequestStateFailed {
				return false
			}

			switch request.Status {
			case api.AWSSpotRequestStatusNotAvailable:
				fallthrough
			case api.AWSSpotRequestStatusNotFulfillable:
				fallthrough
			case api.AWSSpotRequestStatusOversubscribed:
				fallthrough
			case api.AWSSpotRequestStatusPriceToLow:
				return false
			}
		}
	}

	return true
}

type asgStatusCache struct {
	asgNames []api.AWSAsgName
	cache    map[api.AWSAsgName]*asgSpotStatus
	mux      sync.RWMutex
}

func (c *asgStatusCache) asgNameList() []api.AWSAsgName {
	c.mux.RLock()
	defer c.mux.RUnlock()
	return c.asgNames
}

func (c *asgStatusCache) exists(asgName api.AWSAsgName) bool {
	c.mux.RLock()
	_, ok := c.cache[asgName]
	c.mux.RUnlock()

	return ok
}

func (c *asgStatusCache) get(asgName api.AWSAsgName) *asgSpotStatus {
	c.mux.RLock()
	if asgStatus, exists := c.cache[asgName]; exists {
		return asgStatus
	}
	c.mux.RUnlock()

	return nil
}

func (c *asgStatusCache) add(asgName api.AWSAsgName, status *asgSpotStatus) {
	c.mux.Lock()
	if _, exists := c.cache[asgName]; !exists {
		c.asgNames = append(c.asgNames, asgName)
	}
	c.cache[asgName] = status
	c.mux.Unlock()
}

func (c *asgStatusCache) update(asgName api.AWSAsgName, status bool) {
	c.mux.Lock()
	if _, exists := c.cache[asgName]; exists {
		c.cache[asgName].Available = status
		c.cache[asgName].statusChangeTime = time.Now()
	}
	c.mux.Unlock()
}

type spotRequestCache struct {
	createTime time.Time
	cache      []*api.SpotRequest
	mux        sync.RWMutex
}

func (c *spotRequestCache) refresh(requests []*api.SpotRequest) {
	c.mux.Lock()
	c.cache = requests
	c.createTime = time.Now()
	c.mux.Unlock()
}

func (c *spotRequestCache) findRequests(iamInstanceProfile api.AWSIamInstanceProfile, instanceType api.AWSInstanceType) []*api.SpotRequest {
	c.mux.RLock()
	requests := make([]*api.SpotRequest, len(c.cache))

	for _, request := range c.cache {
		if iamInstanceProfile == request.InstanceProfile && instanceType == request.InstanceType {

		}
	}

	c.mux.RUnlock()
	return requests
}
