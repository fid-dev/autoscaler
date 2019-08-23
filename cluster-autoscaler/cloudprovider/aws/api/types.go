package api

// AWSInstanceType describes the instance type (m4.2xlarge, ...)
type AWSInstanceType string

// AWSPriceDimension
type AWSPriceDimension string

// AWSRegion
type AWSRegion string

// AWSAvailabilityZone
type AWSAvailabilityZone string

// AWSSpotRequestID
type AWSSpotRequestID string

// AWSSpotRequestState
type AWSSpotRequestState string

// AWSSpotRequestStatus
type AWSSpotRequestStatus string

// AWSIamInstanceProfile
type AWSIamInstanceProfile string

// AWSLaunchConfigurationName
type AWSLaunchConfigurationName string

// AWSAsgName
type AWSAsgName string

const (
	// AWSSpotRequestStateOpen
	AWSSpotRequestStateOpen = AWSSpotRequestState("open")
	// AWSSpotRequestStateFailed
	AWSSpotRequestStateFailed = AWSSpotRequestState("failed")
	// AWSSpotRequestStatusNotAvailable there is not enough capacity available for the instances that you requested
	AWSSpotRequestStatusNotAvailable = AWSSpotRequestStatus("capacity-not-available")
	// AWSSpotRequestStatusOversubscribed there is not enough capacity available for the instances that you requested
	AWSSpotRequestStatusOversubscribed = AWSSpotRequestStatus("capacity-oversubscribed")
	// AWSSpotRequestStatusPriceToLow
	AWSSpotRequestStatusPriceToLow = AWSSpotRequestStatus("price-too-low")
	// AWSSpotRequestStatusNotFulfillable the Spot request can't be fulfilled because one or more constraints are not valid
	// (for example, the Availability Zone does not exist). The status message indicates which constraint is not valid
	AWSSpotRequestStatusNotFulfillable = AWSSpotRequestStatus("constraint-not-fulfillable")
)
