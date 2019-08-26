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
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/stretchr/testify/assert"
)

func TestSpotRequestManager_List(t *testing.T) {
	cases := []struct {
		name          string
		requests      []*ec2.SpotInstanceRequest
		expected      []*SpotRequest
		expectedError string
		error         string
	}{
		{
			name:          "error fetching list: handle error",
			requests:      []*ec2.SpotInstanceRequest{},
			expected:      []*SpotRequest{},
			expectedError: "",
			error:         "AWS died",
		},
		{
			name:     "no request exists: returns empty list",
			requests: []*ec2.SpotInstanceRequest{},
			expected: []*SpotRequest{},
		},
		{
			name: "no request is young enough: returns empty list",
			requests: []*ec2.SpotInstanceRequest{
				newSpotInstanceRequestInstance("1", "open", "capacity-not-available",
					"123", "m4.2xlarge", "eu-west-1a", &time.Time{}),
			},
			expected: []*SpotRequest{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := NewAwsEC2SpotRequestManagerMock(c.requests)
			mock.setError(c.error)

			service := NewEC2SpotRequestManager(mock)
			list, err := service.List()

			if len(c.error) > 0 {
				assert.Nil(t, list, c.name, "request list should be nil")
				assert.NotNil(t, err, c.name, "awaits an error")

				if err != nil {
					assert.Equal(t, c.expectedError, err.Error(), c.name, "unexpected error")
				}
			} else {
				assert.Nil(t, err, c.name, "no error should have append")
				assert.NotNil(t, list, c.name, "awaits a list")

				if list != nil {
					assert.Equal(t, c.expected, list, c.name, "return list is not valid")
				}
			}
		})
	}
}

var _ AwsEC2SpotRequestManager = &awsEC2SpotRequestManagerMock{}

func NewAwsEC2SpotRequestManagerMock(requests []*ec2.SpotInstanceRequest) *awsEC2SpotRequestManagerMock {
	return &awsEC2SpotRequestManagerMock{requests, ""}
}

type awsEC2SpotRequestManagerMock struct {
	requests []*ec2.SpotInstanceRequest
	error    string
}

func (m *awsEC2SpotRequestManagerMock) setError(errorMessage string) {
	m.error = errorMessage
}

func (m *awsEC2SpotRequestManagerMock) CancelSpotInstanceRequests(input *ec2.CancelSpotInstanceRequestsInput) (*ec2.CancelSpotInstanceRequestsOutput, error) {
	if len(m.error) > 0 {
		return nil, errors.New(m.error)
	}

	canceledIds := make([]*ec2.CancelledSpotInstanceRequest, len(m.requests))

	for _, id := range input.SpotInstanceRequestIds {
		for _, request := range m.requests {
			if aws.StringValue(id) == aws.StringValue(request.SpotInstanceRequestId) {
				canceledIds = append(canceledIds, &ec2.CancelledSpotInstanceRequest{
					SpotInstanceRequestId: request.SpotInstanceRequestId,
					State:                 request.State,
				})
				request.State = aws.String("cancelled")
			}
		}
	}

	return &ec2.CancelSpotInstanceRequestsOutput{CancelledSpotInstanceRequests: canceledIds}, nil
}

func (m *awsEC2SpotRequestManagerMock) DescribeSpotInstanceRequests(input *ec2.DescribeSpotInstanceRequestsInput) (*ec2.DescribeSpotInstanceRequestsOutput, error) {
	if len(m.error) > 0 {
		return nil, errors.New(m.error)
	}

	startTime := time.Time{}
	searchedStates := make([]*string, 0)

	for _, filter := range input.Filters {
		switch aws.StringValue(filter.Name) {
		case "valid-from":
			startTime, _ = time.Parse(aws.StringValue(filter.Values[0]), time.RFC3339)
		case "state":
			for _, state := range filter.Values {
				searchedStates = append(searchedStates, state)
			}
		}
	}

	requests := make([]*ec2.SpotInstanceRequest, len(m.requests))

	for _, request := range m.requests {
		if aws.TimeValue(request.CreateTime).After(startTime) {
			for _, state := range searchedStates {
				if request.State == state {
					requests = append(requests, request)
				}
			}
		}
	}

	return &ec2.DescribeSpotInstanceRequestsOutput{SpotInstanceRequests: requests}, nil
}

func newSpotInstanceRequestInstance(id, state, status, iamInstanceProfile, instanceType, availabilityZone string, created *time.Time) *ec2.SpotInstanceRequest {
	if created == nil {
		created = aws.Time(time.Now())
	}

	return &ec2.SpotInstanceRequest{
		SpotInstanceRequestId:    aws.String(id),
		LaunchedAvailabilityZone: aws.String(availabilityZone),
		LaunchSpecification: &ec2.LaunchSpecification{
			IamInstanceProfile: &ec2.IamInstanceProfileSpecification{
				Name: aws.String(iamInstanceProfile),
			},
			InstanceType: aws.String(instanceType),
		},
		State: aws.String(state),
		Status: &ec2.SpotInstanceStatus{
			Code: aws.String(status),
		},
		CreateTime: created,
	}
}
