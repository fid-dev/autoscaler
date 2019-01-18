/*
Copyright 2018 The Kubernetes Authors.

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

package ess

import (
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/alicloud/alibaba-cloud-sdk-go/sdk/requests"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/alicloud/alibaba-cloud-sdk-go/sdk/responses"
)

// ExecuteScalingRule invokes the ess.ExecuteScalingRule API synchronously
// api document: https://help.aliyun.com/api/ess/executescalingrule.html
func (client *Client) ExecuteScalingRule(request *ExecuteScalingRuleRequest) (response *ExecuteScalingRuleResponse, err error) {
	response = CreateExecuteScalingRuleResponse()
	err = client.DoAction(request, response)
	return
}

// ExecuteScalingRuleWithChan invokes the ess.ExecuteScalingRule API asynchronously
// api document: https://help.aliyun.com/api/ess/executescalingrule.html
// asynchronous document: https://help.aliyun.com/document_detail/66220.html
func (client *Client) ExecuteScalingRuleWithChan(request *ExecuteScalingRuleRequest) (<-chan *ExecuteScalingRuleResponse, <-chan error) {
	responseChan := make(chan *ExecuteScalingRuleResponse, 1)
	errChan := make(chan error, 1)
	err := client.AddAsyncTask(func() {
		defer close(responseChan)
		defer close(errChan)
		response, err := client.ExecuteScalingRule(request)
		if err != nil {
			errChan <- err
		} else {
			responseChan <- response
		}
	})
	if err != nil {
		errChan <- err
		close(responseChan)
		close(errChan)
	}
	return responseChan, errChan
}

// ExecuteScalingRuleWithCallback invokes the ess.ExecuteScalingRule API asynchronously
// api document: https://help.aliyun.com/api/ess/executescalingrule.html
// asynchronous document: https://help.aliyun.com/document_detail/66220.html
func (client *Client) ExecuteScalingRuleWithCallback(request *ExecuteScalingRuleRequest, callback func(response *ExecuteScalingRuleResponse, err error)) <-chan int {
	result := make(chan int, 1)
	err := client.AddAsyncTask(func() {
		var response *ExecuteScalingRuleResponse
		var err error
		defer close(result)
		response, err = client.ExecuteScalingRule(request)
		callback(response, err)
		result <- 1
	})
	if err != nil {
		defer close(result)
		callback(nil, err)
		result <- 0
	}
	return result
}

// ExecuteScalingRuleRequest is the request struct for api ExecuteScalingRule
type ExecuteScalingRuleRequest struct {
	*requests.RpcRequest
	ResourceOwnerId      requests.Integer `position:"Query" name:"ResourceOwnerId"`
	ScalingRuleAri       string           `position:"Query" name:"ScalingRuleAri"`
	ResourceOwnerAccount string           `position:"Query" name:"ResourceOwnerAccount"`
	ClientToken          string           `position:"Query" name:"ClientToken"`
	OwnerAccount         string           `position:"Query" name:"OwnerAccount"`
	OwnerId              requests.Integer `position:"Query" name:"OwnerId"`
}

// ExecuteScalingRuleResponse is the response struct for api ExecuteScalingRule
type ExecuteScalingRuleResponse struct {
	*responses.BaseResponse
	ScalingActivityId string `json:"ScalingActivityId" xml:"ScalingActivityId"`
	RequestId         string `json:"RequestId" xml:"RequestId"`
}

// CreateExecuteScalingRuleRequest creates a request to invoke ExecuteScalingRule API
func CreateExecuteScalingRuleRequest() (request *ExecuteScalingRuleRequest) {
	request = &ExecuteScalingRuleRequest{
		RpcRequest: &requests.RpcRequest{},
	}
	request.InitWithApiInfo("Ess", "2014-08-28", "ExecuteScalingRule", "ess", "openAPI")
	return
}

// CreateExecuteScalingRuleResponse creates a response to parse from ExecuteScalingRule response
func CreateExecuteScalingRuleResponse() (response *ExecuteScalingRuleResponse) {
	response = &ExecuteScalingRuleResponse{
		BaseResponse: &responses.BaseResponse{},
	}
	return
}
