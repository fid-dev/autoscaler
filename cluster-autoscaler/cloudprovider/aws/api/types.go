package api

type AWSInstanceType string
type AWSPriceDimension string
type AWSRegion string
type AWSAvailabilityZone string
type AWSSpotRequestID string
type AWSSpotRequestState string
type AWSSpotRequestStatus string
type AWSIamInstanceProfile string
type AWSLaunchConfigurationName string
type AWSAsgName string

const (
	AWSSpotRequestStateFailed          = AWSSpotRequestState("failed")
	AWSSpotRequestStatusNotAvailable   = AWSSpotRequestStatus("capacity-not-available")
	AWSSpotRequestStatusOversubscribed = AWSSpotRequestStatus("capacity-oversubscribed")
	AWSSpotRequestStatusPriceToLow     = AWSSpotRequestStatus("price-too-low")
	AWSSpotRequestStatusNotFulfillable = AWSSpotRequestStatus("constraint-not-fulfillable")
)
