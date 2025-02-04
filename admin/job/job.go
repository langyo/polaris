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

package job

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/polarismesh/polaris/cache"
	commonlog "github.com/polarismesh/polaris/common/log"
	"github.com/polarismesh/polaris/common/utils"
	"github.com/polarismesh/polaris/service"
	"github.com/polarismesh/polaris/store"
)

var log = commonlog.GetScopeOrDefaultByName(commonlog.DefaultLoggerName)

// MaintainJobs
type MaintainJobs struct {
	jobs        map[string]maintainJob
	startedJobs map[string]maintainJob
	storage     store.Store
	cancel      context.CancelFunc
}

// NewMaintainJobs
func NewMaintainJobs(namingServer service.DiscoverServer, cacheMgn *cache.CacheManager,
	storage store.Store) *MaintainJobs {
	return &MaintainJobs{
		jobs: map[string]maintainJob{
			"DeleteUnHealthyInstance": &deleteUnHealthyInstanceJob{
				namingServer: namingServer, storage: storage},
			"DeleteEmptyAutoCreatedService": &deleteEmptyAutoCreatedServiceJob{
				namingServer: namingServer, cacheMgn: cacheMgn, storage: storage},
			"CleanDeletedInstances": &cleanDeletedInstancesJob{
				storage: storage},
			"CleanDeletedClients": &cleanDeletedClientsJob{
				storage: storage},
		},
		startedJobs: map[string]maintainJob{},
		storage:     storage,
	}
}

// StartMaintainJobs
func (mj *MaintainJobs) StartMaintianJobs(configs []JobConfig) error {
	ctx, cancel := context.WithCancel(context.Background())
	mj.cancel = cancel
	for _, cfg := range configs {
		if !cfg.Enable {
			log.Infof("[Maintain][Job] job (%s) not enable", cfg.Name)
			continue
		}
		job, ok := mj.jobs[cfg.Name]
		if !ok {
			return fmt.Errorf("[Maintain][Job] job (%s) not exist", cfg.Name)
		}
		_, ok = mj.startedJobs[cfg.Name]
		if ok {
			return fmt.Errorf("[Maintain][Job] job (%s) duplicated", cfg.Name)
		}
		err := job.init(cfg.Option)
		if err != nil {
			log.Errorf("[Maintain][Job] job (%s) fail to init, err: %v", cfg.Name, err)
			return fmt.Errorf("[Maintain][Job] job (%s) fail to init", cfg.Name)
		}
		err = mj.storage.StartLeaderElection(store.ElectionKeyMaintainJobPrefix + cfg.Name)
		if err != nil {
			log.Errorf("[Maintain][Job][%s] start leader election err: %v", cfg.Name, err)
			return err
		}
		runAdminJob(ctx, cfg.Name, job.interval(), job, mj.storage)
		mj.startedJobs[cfg.Name] = job
	}
	return nil
}

// StopMaintainJobs
func (mj *MaintainJobs) StopMaintainJobs() {
	if mj.cancel != nil {
		mj.cancel()
	}
	mj.startedJobs = map[string]maintainJob{}
}

func runAdminJob(ctx context.Context, name string, interval time.Duration, job maintainJob, storage store.Store) {
	f := func() {
		if !storage.IsLeader(store.ElectionKeyMaintainJobPrefix + name) {
			log.Infof("[Maintain][Job][%s] I am follower", name)
			job.clear()
			return
		}
		log.Infof("[Maintain][Job][%s] I am leader, job start", name)
		job.execute()
		log.Infof("[Maintain][Job][%s] I am leader, job end", name)
	}

	ticker := time.NewTicker(interval)
	go func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				f()
			}
		}
	}(ctx)
}

type maintainJob interface {
	init(cfg map[string]interface{}) error
	execute()
	clear()
	interval() time.Duration
}

func getMasterAccountToken(storage store.Store) (string, error) {
	mainUser := os.Getenv("POLARIS_MAIN_USER")
	if mainUser == "" {
		mainUser = "polaris"
	}
	user, err := storage.GetUserByName(mainUser, "")
	if err != nil {
		return "", err
	}
	return user.Token, nil
}

func buildContext(storage store.Store) (context.Context, error) {
	token, err := getMasterAccountToken(storage)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, utils.ContextAuthTokenKey, token)
	ctx = context.WithValue(ctx, utils.ContextOperator, "maintain-job")
	return ctx, nil
}
