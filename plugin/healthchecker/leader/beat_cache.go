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

package leader

import (
	"strconv"

	apiservice "github.com/polarismesh/specification/source/go/api/v1/service_manage"
	"go.uber.org/zap"

	"github.com/polarismesh/polaris/common/utils"
)

// ReadBeatRecord Heartbeat records read results
type ReadBeatRecord struct {
	Record RecordValue
	Exist  bool
}

// WriteBeatRecord Heartbeat record operation results
type WriteBeatRecord struct {
	Record RecordValue
	Key    string
}

// RecordValue heatrtbeat record value
type RecordValue struct {
	Server     string
	CurTimeSec int64
	Count      int64
}

func (r RecordValue) String() string {
	secStr := strconv.FormatInt(r.CurTimeSec, 10)
	countStr := strconv.FormatInt(r.Count, 10)
	return r.Server + Split + secStr + Split + countStr
}

type (
	// HashFunction hash function to caul record id need locate in SegmentMap
	HashFunction func(string) int
	// RecordSaver beat record saver
	RecordSaver func(req *apiservice.HeartbeatsRequest)
	// RecordDelter beat record delter
	RecordDelter func(req *apiservice.DelHeartbeatsRequest)
	// RecordGetter beat record getter
	RecordGetter func(req *apiservice.GetHeartbeatsRequest) *apiservice.GetHeartbeatsResponse
	// BeatRecordCache Heartbeat data cache
	BeatRecordCache interface {
		// Get get records
		Get(keys ...string) map[string]*ReadBeatRecord
		// Put put records
		Put(records ...WriteBeatRecord)
		// Del del records
		Del(keys ...string)
		// Clean .
		Clean()
		// Snapshot
		Snapshot() map[string]*ReadBeatRecord
	}
)

// newLocalBeatRecordCache
func newLocalBeatRecordCache(soltNum int32, hashFunc HashFunction) BeatRecordCache {
	if soltNum == 0 {
		soltNum = DefaultSoltNum
	}
	return &LocalBeatRecordCache{
		soltNum:  soltNum,
		hashFunc: hashFunc,
		beatCache: utils.NewSegmentMap[string, RecordValue](int(soltNum), func(k string) int {
			return hashFunc(k)
		}),
	}
}

// LocalBeatRecordCache
type LocalBeatRecordCache struct {
	soltNum   int32
	hashFunc  HashFunction
	beatCache *utils.SegmentMap[string, RecordValue]
}

func (lc *LocalBeatRecordCache) Get(keys ...string) map[string]*ReadBeatRecord {
	ret := make(map[string]*ReadBeatRecord, len(keys))
	for i := range keys {
		key := keys[i]
		val, ok := lc.beatCache.Get(key)
		ret[key] = &ReadBeatRecord{
			Record: val,
			Exist:  ok,
		}
	}
	return ret
}

func (lc *LocalBeatRecordCache) Put(records ...WriteBeatRecord) {
	for i := range records {
		record := records[i]
		if log.DebugEnabled() {
			plog.Debug("receive put action", zap.Any("record", record))
		}
		lc.beatCache.Put(record.Key, record.Record)
	}
}

func (lc *LocalBeatRecordCache) Del(keys ...string) {
	for i := range keys {
		key := keys[i]
		ok := lc.beatCache.Del(key)
		if log.DebugEnabled() {
			plog.Debug("delete result", zap.String("key", key), zap.Bool("exist", ok))
		}
	}
}

func (lc *LocalBeatRecordCache) Clean() {
	// do nothing
}

func (lc *LocalBeatRecordCache) Snapshot() map[string]*ReadBeatRecord {
	ret := map[string]*ReadBeatRecord{}
	lc.beatCache.Range(func(k string, v RecordValue) {
		ret[k] = &ReadBeatRecord{
			Record: v,
		}
	})
	return ret
}

// newRemoteBeatRecordCache
func newRemoteBeatRecordCache(getter RecordGetter, saver RecordSaver,
	delter RecordDelter) BeatRecordCache {
	return &RemoteBeatRecordCache{
		getter: getter,
		saver:  saver,
		delter: delter,
	}
}

// RemoteBeatRecordCache
type RemoteBeatRecordCache struct {
	saver  RecordSaver
	delter RecordDelter
	getter RecordGetter
}

func (rc *RemoteBeatRecordCache) Get(keys ...string) map[string]*ReadBeatRecord {
	ret := make(map[string]*ReadBeatRecord)
	for i := range keys {
		ret[keys[i]] = &ReadBeatRecord{
			Exist: false,
		}
	}
	resp := rc.getter(&apiservice.GetHeartbeatsRequest{
		InstanceIds: keys,
	})
	records := resp.GetRecords()
	for i := range records {
		record := records[i]
		val, ok := ret[record.InstanceId]
		if !ok {
			val.Exist = false
			continue
		}
		val.Exist = record.GetExist()
		val.Record = RecordValue{
			CurTimeSec: record.GetLastHeartbeatSec(),
		}
	}
	return ret
}

func (rc *RemoteBeatRecordCache) Put(records ...WriteBeatRecord) {
	req := &apiservice.HeartbeatsRequest{
		Heartbeats: make([]*apiservice.InstanceHeartbeat, 0, len(records)),
	}
	for i := range records {
		record := records[i]
		req.Heartbeats = append(req.Heartbeats, &apiservice.InstanceHeartbeat{
			InstanceId: record.Key,
		})
	}
	rc.saver(req)
}

func (rc *RemoteBeatRecordCache) Del(key ...string) {
	req := &apiservice.DelHeartbeatsRequest{
		InstanceIds: key,
	}
	rc.delter(req)
}

func (lc *RemoteBeatRecordCache) Clean() {
	// do nothing
}

func (lc *RemoteBeatRecordCache) Snapshot() map[string]*ReadBeatRecord {
	return map[string]*ReadBeatRecord{}
}
