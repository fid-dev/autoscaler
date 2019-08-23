package api

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
)

type AwsEC2SpotRequestLister interface {
	DescribeSpotInstanceRequests(input *ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error)
}

type SpotRequest struct {
	ID               AWSSpotRequestID
	InstanceProfile  AWSIamInstanceProfile
	AvailabilityZone AWSAvailabilityZone
	InstanceType     AWSInstanceType
	State            AWSSpotRequestState
	Status           AWSSpotRequestStatus
}

type SpotRequestService interface {
	List() ([]*SpotRequest, error)
}

var _ SpotRequestService = &spotRequestService{}

// NewEC2SpotRequestService is the constructor of spotRequestService which is a wrapper for the AWS EC2 API
func NewEC2SpotRequestService(awsEC2Service AwsEC2SpotRequestLister) *spotRequestService {
	return &spotRequestService{
		service:       awsEC2Service,
		lastCheckTime: time.Time{},
	}
}

type spotRequestService struct {
	service       AwsEC2SpotRequestLister
	lastCheckTime time.Time
}

func (srs *spotRequestService) List() ([]*SpotRequest, error) {
	list := make([]*SpotRequest, 0)

	arguments := srs.listArguments()

	awsSpotRequests, err := srs.service.DescribeSpotInstanceRequests(arguments)
	if err != nil {
		return nil, errors.Wrap(err, "could not retrieve AWS Spot Request list")
	}

	for _, request := range awsSpotRequests.SpotInstanceRequests {
		converted := srs.convertAwsSpotRequest(request)
		list = append(list, converted)
	}

	srs.lastCheckTime = time.Now()

	return list, nil
}

func (srs *spotRequestService) convertAwsSpotRequest(request *ec2.SpotInstanceRequest) *SpotRequest {
	converted := new(SpotRequest)

	converted.ID = AWSSpotRequestID(aws.StringValue(request.SpotInstanceRequestId))
	converted.AvailabilityZone = AWSAvailabilityZone(aws.StringValue(request.LaunchedAvailabilityZone))
	converted.InstanceProfile = AWSIamInstanceProfile(aws.StringValue(request.LaunchSpecification.IamInstanceProfile.Name))
	converted.InstanceType = AWSInstanceType(aws.StringValue(request.LaunchSpecification.InstanceType))
	converted.State = AWSSpotRequestState(aws.StringValue(request.State))
	converted.Status = AWSSpotRequestStatus(aws.StringValue(request.Status.Code))

	return converted
}

func (srs *spotRequestService) listArguments() *ec2.DescribeSpotInstanceRequestsInput {
	arguments := &ec2.DescribeSpotInstanceRequestsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("valid-from"),
				Values: []*string{
					aws.String(srs.lastCheckTime.Format(time.RFC3339)),
				},
			},
			{
				Name: aws.String("state"),
				Values: []*string{
					aws.String("open"),
					aws.String("failed"),
				},
			},
		},
	}

	return arguments
}

func (srs *spotRequestService) clusterNameFromRequest(request *ec2.SpotInstanceRequest) string {
	return ""
}
