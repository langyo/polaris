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

package heartbeatmemory

import (
	"sync"
	"sync/atomic"

	commonLog "github.com/polarismesh/polaris/common/log"
	commontime "github.com/polarismesh/polaris/common/time"
	"github.com/polarismesh/polaris/plugin"
)

// 把操作记录记录到日志文件中
const (
	// PluginName plugin name
	PluginName = "heartbeatMemory"
)

var log = commonLog.GetScopeOrDefaultByName(commonLog.HealthcheckLoggerName)

// HeartbeatRecord record for heartbeat
type HeartbeatRecord struct {
	Server     string
	CurTimeSec int64
	Count      int64
}

// MemoryHealthChecker memory health checker
type MemoryHealthChecker struct {
	hbRecords      *sync.Map
	suspendTimeSec int64
}

// Name return plugin name
func (r *MemoryHealthChecker) Name() string {
	return PluginName
}

// Initialize initialize plugin
func (r *MemoryHealthChecker) Initialize(c *plugin.ConfigEntry) error {
	r.hbRecords = &sync.Map{}
	return nil
}

// Destroy plugin destruction
func (r *MemoryHealthChecker) Destroy() error {
	return nil
}

// Type for health check plugin, only one same type plugin is allowed
func (r *MemoryHealthChecker) Type() plugin.HealthCheckType {
	return plugin.HealthCheckerHeartbeat
}

// Report process heartbeat info report
func (r *MemoryHealthChecker) Report(request *plugin.ReportRequest) error {
	record := HeartbeatRecord{
		Server:     request.LocalHost,
		CurTimeSec: request.CurTimeSec,
		Count:      request.Count,
	}
	r.hbRecords.Store(request.InstanceId, record)
	log.Debugf("[HealthCheck][MemoryCheck]add hb record, instanceId %s, record %+v", request.InstanceId, record)
	return nil
}

// Query queries the heartbeat time
func (r *MemoryHealthChecker) Query(request *plugin.QueryRequest) (*plugin.QueryResponse, error) {
	recordValue, ok := r.hbRecords.Load(request.InstanceId)
	if !ok {
		return &plugin.QueryResponse{
			LastHeartbeatSec: 0,
		}, nil
	}
	record := recordValue.(HeartbeatRecord)
	log.Debugf("[HealthCheck][MemoryCheck]query hb record, instanceId %s, record %+v", request.InstanceId, record)
	return &plugin.QueryResponse{
		Server:           record.Server,
		LastHeartbeatSec: record.CurTimeSec,
		Count:            record.Count,
	}, nil
}

func (r *MemoryHealthChecker) skipCheck(instanceId string, expireDurationSec int64) bool {
	suspendTimeSec := r.SuspendTimeSec()
	localCurTimeSec := commontime.CurrentMillisecond() / 1000
	if suspendTimeSec > 0 && localCurTimeSec >= suspendTimeSec && localCurTimeSec-suspendTimeSec < expireDurationSec {
		log.Infof("[Health Check][MemoryCheck]health check redis suspended, "+
			"suspendTimeSec is %d, localCurTimeSec is %d, expireDurationSec is %d, instanceId %s",
			suspendTimeSec, localCurTimeSec, expireDurationSec, instanceId)
		return true
	}
	return false
}

// Check Report process the instance check
func (r *MemoryHealthChecker) Check(request *plugin.CheckRequest) (*plugin.CheckResponse, error) {
	queryResp, err := r.Query(&request.QueryRequest)
	if err != nil {
		return nil, err
	}
	lastHeartbeatTime := queryResp.LastHeartbeatSec
	checkResp := &plugin.CheckResponse{
		LastHeartbeatTimeSec: lastHeartbeatTime,
	}
	curTimeSec := request.CurTimeSec()
	log.Debugf("[HealthCheck][MemoryCheck]check hb record, cur is %d, last is %d", curTimeSec, lastHeartbeatTime)
	if r.skipCheck(request.InstanceId, int64(request.ExpireDurationSec)) {
		checkResp.StayUnchanged = true
		return checkResp, nil
	}
	if curTimeSec > lastHeartbeatTime {
		if curTimeSec-lastHeartbeatTime >= int64(request.ExpireDurationSec) {
			// 心跳超时
			checkResp.Healthy = false

			if request.Healthy {
				log.Infof("[Health Check][MemoryCheck]health check expired, "+
					"last hb timestamp is %d, curTimeSec is %d, expireDurationSec is %d, instanceId %s",
					lastHeartbeatTime, curTimeSec, request.ExpireDurationSec, request.InstanceId)
			} else {
				checkResp.StayUnchanged = true
			}
			return checkResp, nil
		}
	}
	checkResp.Healthy = true
	if !request.Healthy {
		log.Infof("[Health Check][MemoryCheck]health check resumed, "+
			"last hb timestamp is %d, curTimeSec is %d, expireDurationSec is %d instanceId %s",
			lastHeartbeatTime, curTimeSec, request.ExpireDurationSec, request.InstanceId)
	} else {
		checkResp.StayUnchanged = true
	}

	return checkResp, nil
}

// AddToCheck add the instances to check procedure
func (r *MemoryHealthChecker) AddToCheck(request *plugin.AddCheckRequest) error {
	return nil
}

// RemoveFromCheck removes the instances from check procedure
func (r *MemoryHealthChecker) RemoveFromCheck(request *plugin.AddCheckRequest) error {
	return nil
}

// Delete delete the id
func (r *MemoryHealthChecker) Delete(id string) error {
	r.hbRecords.Delete(id)
	return nil
}

func (r *MemoryHealthChecker) Suspend() {
	curTimeMilli := commontime.CurrentMillisecond() / 1000
	log.Infof("[Health Check][MemoryCheck] suspend checker, start time %d", curTimeMilli)
	atomic.StoreInt64(&r.suspendTimeSec, curTimeMilli)
}

// SuspendTimeSec get suspend time in seconds
func (r *MemoryHealthChecker) SuspendTimeSec() int64 {
	return atomic.LoadInt64(&r.suspendTimeSec)
}

func (r *MemoryHealthChecker) DebugHandlers() []plugin.DebugHandler {
	return []plugin.DebugHandler{}
}

func init() {
	d := &MemoryHealthChecker{}
	plugin.RegisterPlugin(d.Name(), d)
}
