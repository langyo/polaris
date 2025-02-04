/**
 * Tencent is pleased to support the open source community by making Polaris available.
 *
 * Copyright (C) 2019 THL A29 Limited, a Tencent company. All rights reserved.
 *
 * Licensed under the BSD 3-Clause License (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * https://opensource.org/licenses/BSD-3-Clause
 *
 * Unless required by applicable law or agreed to in writing, software distributed
 * under the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
 * CONDITIONS OF ANY KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations under the License.
 */

package v1

import (
	"context"

	apiservice "github.com/polarismesh/specification/source/go/api/v1/service_manage"

	"github.com/polarismesh/polaris/plugin"
)

// BatchGetHeartbeat 批量获取心跳记录
func (g *DiscoverServer) BatchGetHeartbeat(ctx context.Context,
	req *apiservice.GetHeartbeatsRequest) (*apiservice.GetHeartbeatsResponse, error) {
	checker, ok := g.healthCheckServer.Checkers()[int32(apiservice.HealthCheck_HEARTBEAT)]
	if !ok {
		return &apiservice.GetHeartbeatsResponse{}, nil
	}
	keys := req.GetInstanceIds()
	records := make([]*apiservice.HeartbeatRecord, 0, len(keys))
	for i := range keys {
		resp, err := checker.Query(&plugin.QueryRequest{
			InstanceId: keys[i],
		})
		if err != nil {
			return nil, err
		}
		record := &apiservice.HeartbeatRecord{
			InstanceId:       keys[i],
			LastHeartbeatSec: resp.LastHeartbeatSec,
			Exist:            resp.Exists,
		}
		records = append(records, record)
	}
	return &apiservice.GetHeartbeatsResponse{
		Records: records,
	}, nil
}

// BatchDelHeartbeat 批量删除心跳记录
func (g *DiscoverServer) BatchDelHeartbeat(ctx context.Context,
	req *apiservice.DelHeartbeatsRequest) (*apiservice.DelHeartbeatsResponse, error) {
	checker, ok := g.healthCheckServer.Checkers()[int32(apiservice.HealthCheck_HEARTBEAT)]
	if !ok {
		return &apiservice.DelHeartbeatsResponse{}, nil
	}
	keys := req.GetInstanceIds()
	for i := range keys {
		if err := checker.Delete(keys[i]); err != nil {
			return nil, err
		}
	}
	return &apiservice.DelHeartbeatsResponse{}, nil
}
