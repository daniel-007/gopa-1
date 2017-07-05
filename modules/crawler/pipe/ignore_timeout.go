/*
Copyright 2016 Medcl (m AT medcl.net)

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

package pipe

import (
	log "github.com/cihub/seelog"
	"github.com/infinitbyte/gopa/core/model"
	. "github.com/infinitbyte/gopa/core/pipeline"
	"github.com/infinitbyte/gopa/core/stats"
	"github.com/infinitbyte/gopa/modules/config"
)

const IgnoreTimeout JointKey = "ignore_timeout"

func (this IgnoreTimeoutJoint) Name() string {
	return string(IgnoreTimeout)
}

type IgnoreTimeoutJoint struct {
	IgnoreTimeoutAfterCount int64
}

func (this IgnoreTimeoutJoint) Process(context *Context) error {

	task := context.MustGet(CONTEXT_CRAWLER_TASK).(*model.Task)

	//TODO ignore within time period, rather than total count
	host := task.Host
	timeoutCount := stats.Stat("domain.stats", host+"."+config.STATS_FETCH_TIMEOUT_COUNT)
	if timeoutCount > this.IgnoreTimeoutAfterCount {
		stats.Increment("domain.stats", host+"."+config.STATS_FETCH_TIMEOUT_IGNORE_COUNT)
		context.Break("too much timeout on this domain, ignored " + host)
		log.Warnf("hit timeout host, %s , ignore after,%d ", host, timeoutCount)
	}
	return nil
}
