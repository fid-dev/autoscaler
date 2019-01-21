/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/pricing"
	"github.com/golang/glog"
	"github.com/pkg/errors"
)

const (
	instanceInfoCacheMaxAge      = time.Hour * 6
	awsPricingAPIURLTemplate     = "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/%s/index.json"
	instanceOperatingSystemLinux = "Linux"
	instanceTenancyShared        = "Shared"
)

// TODO <ylallemant> find some API for this map - support case opened
var (
	azIdRegexp    = regexp.MustCompile(`(\d+)$`)
	regionNameMap = map[string]string{
		"USA Ost (Ohio)":             "us-east-2",
		"USA Ost (Nord-Virginia)":    "us-east-1",
		"USA West (Nordkalifornien)": "us-west-1",
		"USA West (Oregon)":          "us-west-2",
		"Asia Pacific (Mumbai)":      "ap-south-1",
		"Asia Pacific (Osaka-Local)": "ap-northeast-3",
		"Asia Pacific (Seoul)":       "ap-northeast-2",
		"Asia Pacific (Singapur)":    "ap-southeast-1",
		"Asia Pacific (Sydney)":      "ap-southeast-2",
		"Asia Pacific (Tokio)":       "ap-northeast-1",
		"Canada (Central)":           "ca-central-1",
		"China (Peking)":             "cn-north-1",
		"China (Ningxia)":            "cn-northwest-1",
		"EU (Frankfurt)":             "eu-central-1",
		"EU (Irland)":                "eu-west-1",
		"EU (London)":                "eu-west-2",
		"EU (Paris)":                 "eu-west-3",
		"EU (Stockholm)":             "eu-north-1",
		"South America (São Paulo)":  "sa-east-1",
		"AWS GovCloud (US-East)":     "us-gov-east-1",
		"AWS GovCloud (USA)":         "us-gov-west-1",
	}
)

// InstanceInfo holds AWS EC2 instance information
type InstanceInfo struct {
	// InstanceType of the described instance
	InstanceType string
	// OnDemandPrice in USD of the ec2 instance
	OnDemandPrice float64
	// VCPU count of this instance
	VCPU int64
	// MemoryMb size in megabytes of this instance
	MemoryMb int64
	// GPU count of this instance
	GPU int64
}

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewEC2InstanceInfoService is the constructor of instanceInfoService which is a wrapper for AWS Pricing API.
func NewEC2InstanceInfoService(client httpClient) *instanceInfoService {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
	})
	if err != nil {
		panic(errors.Wrap(err, "could not create AWS session"))
	}

	return &instanceInfoService{
		client: pricing.New(sess),
		cache:  make(instanceInfoCache),
	}
}

type instanceInfoService struct {
	client *pricing.Pricing
	cache  instanceInfoCache
	sync.RWMutex
}

// DescribeInstanceInfo returns the corresponding aws instance info by given instance type and availability zone.
func (s *instanceInfoService) DescribeInstanceInfo(instanceType string, availabilityZone string) (*InstanceInfo, error) {
	if s.shouldSync(availabilityZone) {
		if err := s.sync(availabilityZone); err != nil {
			// TODO <mrcrgl> may this be tolerated for resilience
			return nil, fmt.Errorf("failed to sync aws product and price information: %v", err)
		}
	}

	if bucket, found := s.cache[availabilityZone]; found {
		for _, info := range bucket.info {
			if info.InstanceType == instanceType {
				return &info, nil
			}
		}
	}
	return nil, fmt.Errorf("instance info not available for instance type %s in zone %s", instanceType, availabilityZone)
}

func (s *instanceInfoService) shouldSync(availabilityZone string) bool {
	bucket, found := s.cache[availabilityZone]
	if !found {
		return true
	}

	return bucket.LastSync().Before(time.Now().Truncate(instanceInfoCacheMaxAge))
}

func (s *instanceInfoService) sync(availabilityZone string) error {
	s.Lock()
	defer s.Unlock()

	start := time.Now()

	bucket, found := s.cache[availabilityZone]
	if !found {
		bucket = new(regionalInstanceInfoBucket)
		s.cache[availabilityZone] = bucket
	}

	response, err := s.fetch(availabilityZone, bucket.ETag)
	if err != nil {
		return err
	}

	defer func() {
		glog.V(4).Infof("Synchronized aws ec2 instance information for availability zone %s - took %s", availabilityZone, time.Now().Sub(start).String())
	}()

	if response == nil {
		bucket.SetLastSync()
		return nil
	}

	instances := make([]InstanceInfo, 0)
	now := time.Now()

	for _, product := range response.Products {
		sku := product.SKU
		attr := product.Attributes

		// TODO <mrcrgl> find better solution for the case of windows installations for instance.
		if attr.OperatingSystem != instanceOperatingSystemLinux {
			continue
		}

		// We do actually only support Shared tenancy instances.
		// See for more information: http://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instance-purchasing-options.html
		if attr.Tenancy != instanceTenancyShared {
			continue
		}

		if len(attr.InstanceType) == 0 {
			continue
		}

		i := InstanceInfo{
			InstanceType: attr.InstanceType,
		}

		var err error
		if attr.Memory != "" && attr.Memory != "NA" {
			if i.MemoryMb, err = parseMemory(attr.Memory); err != nil {
				return fmt.Errorf("parser error %v", err)
			}
		}

		if attr.VCPU != "" {
			if i.VCPU, err = parseCPU(attr.VCPU); err != nil {
				return fmt.Errorf("parser error %v", err)
			}
		}
		if attr.GPU != "" {
			if i.GPU, err = parseCPU(attr.GPU); err != nil {
				return fmt.Errorf("parser error %v", err)
			}
		}

		for priceSKU, offers := range response.Terms.OnDemand {
			if priceSKU != sku {
				continue
			}

			var lastOfferTime time.Time
			var lastOfferPrice float64

			for _, offer := range offers {
				if offer.EffectiveDate.After(now) {
					continue
				}

				for _, price := range offer.PriceDimensions {
					if price.EndRange != "Inf" || price.Unit != "Hrs" {
						continue
					}
					p, err := strconv.ParseFloat(price.PricePerUnit.USD, 64)
					if err != nil {
						return fmt.Errorf("error parsing price for SKU %s [%s] %v", sku, price.PricePerUnit.USD, err)
					}

					if p == 0.0 {
						continue
					}

					if lastOfferTime.IsZero() || lastOfferTime.After(offer.EffectiveDate) {
						lastOfferTime = offer.EffectiveDate
						lastOfferPrice = p
					}
				}
			}

			i.OnDemandPrice = lastOfferPrice
		}

		instances = append(instances, i)
	}

	bucket.Clear()
	bucket.Add(instances...)
	bucket.SetLastSync()

	return nil
}

func (s *instanceInfoService) fetch(availabilityZone string, etag string) (*response, error) {
	url := fmt.Sprintf(awsPricingAPIURLTemplate, availabilityZone)
	regionName, err := regionFullName(availabilityZone)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("GET", url, nil)

	if len(etag) != 0 {
		req.Header.Add("If-None-Match", etag)
	}

	input := &pricing.GetProductsInput{
		Filters: []*pricing.Filter{
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("ServiceCode"),
				Value: aws.String("AmazonEC2"),
			},
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("location"),
				Value: aws.String(regionName),
			},
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("capacitystatus"),
				Value: aws.String("Used"),
			},
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("tenancy"),
				Value: aws.String("Shared"),
			},
			{
				Type:  aws.String("TERM_MATCH"),
				Field: aws.String("preInstalledSw"),
				Value: aws.String("NA"),
			},
		},
	}

	output, err := s.client.GetProducts(input)
	if err != nil {
		return nil, errors.Wrapf(err, "could not fetch products for AZ %s", availabilityZone)
	}

	var data = new(response)

	for _, entry := range output.PriceList {
		rawProduct := entry["product"]
		rawTerms := entry["terms"]

		fmt.Println("RAW ENTRY", entry)
		fmt.Println("---------------")
		fmt.Println("RAW PRODUCT", rawProduct)
		fmt.Println("---------------")
		fmt.Println("RAW Terms", rawTerms)

		return nil, errors.New("END OF TEST")
	}

	return data, nil
}

type instanceInfoCache map[string]*regionalInstanceInfoBucket

type regionalInstanceInfoBucket struct {
	sync.RWMutex
	lastSync time.Time
	ETag     string
	info     []InstanceInfo
}

func (b *regionalInstanceInfoBucket) SetLastSync() {
	b.Lock()
	defer b.Unlock()

	b.lastSync = time.Now()
}

func (b *regionalInstanceInfoBucket) LastSync() time.Time {
	b.RLock()
	defer b.RUnlock()

	return b.lastSync
}

func (b *regionalInstanceInfoBucket) Clear() {
	b.Lock()
	defer b.Unlock()

	b.info = make([]InstanceInfo, 0)
}

func (b *regionalInstanceInfoBucket) Add(info ...InstanceInfo) {
	b.Lock()
	defer b.Unlock()

	b.info = append(b.info, info...)
}

type priceListEntry struct {
	Product product `json:"product"`
	Terms   terms   `json:"terms"`
}

type response struct {
	Products map[string]product `json:"products"`
	Terms    terms              `json:"terms"`
}

type terms struct {
	OnDemand map[string]productOffers `json:"OnDemand"`
}

type productOffers map[string]productOffer

type productOffer struct {
	OfferTermCode   string                           `json:"offerTermCode"`
	EffectiveDate   time.Time                        `json:"effectiveDate"`
	SKU             string                           `json:"sku"`
	PriceDimensions map[string]productPriceDimension `json:"priceDimensions"`
}

type productPriceDimension struct {
	RateCode     string       `json:"rateCode"`
	Description  string       `json:"description"`
	Unit         string       `json:"unit"`
	BeginRange   string       `json:"beginRange"`
	EndRange     string       `json:"endRange"`
	PricePerUnit pricePerUnit `json:"pricePerUnit"`
}

type pricePerUnit struct {
	USD string `json:"USD"`
}

type product struct {
	SKU        string            `json:"sku"`
	Attributes productAttributes `json:"attributes"`
}

type productAttributes struct {
	Tenancy         string `json:"tenancy"`
	InstanceType    string `json:"instanceType"`
	VCPU            string `json:"vcpu"`
	Memory          string `json:"memory"`
	GPU             string `json:"gpu"`
	OperatingSystem string `json:"operatingSystem"`
}

func parseMemory(memory string) (int64, error) {
	reg, err := regexp.Compile("[^0-9\\.]+")
	if err != nil {
		return 0, fmt.Errorf("error compiling regex %v", err)
	}

	parsed := strings.TrimSpace(reg.ReplaceAllString(memory, ""))
	mem, err := strconv.ParseFloat(parsed, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing memory [%s] %v", memory, err)
	}

	return int64(mem * float64(1024)), nil
}

func parseCPU(cpu string) (int64, error) {
	i, err := strconv.ParseInt(cpu, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing cpu [%s] %v", cpu, err)
	}
	return i, nil
}

func regionFullName(availabilityzone string) (string, error) {
	regionShortName := azIdRegexp.ReplaceAllString(availabilityzone, "")

	if fullName, ok := regionNameMap[regionShortName]; ok {
		return fullName, nil
	}

	return "", errors.New(fmt.Sprintf("region full name for not found for availability zone: %s and region %s", availabilityzone, regionShortName))
}
