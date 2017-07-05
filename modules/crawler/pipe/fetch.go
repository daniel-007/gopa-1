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
	"fmt"
	log "github.com/cihub/seelog"
	"github.com/infinitbyte/gopa/core/errors"
	"github.com/infinitbyte/gopa/core/model"
	. "github.com/infinitbyte/gopa/core/pipeline"
	"github.com/infinitbyte/gopa/core/queue"
	"github.com/infinitbyte/gopa/core/stats"
	"github.com/infinitbyte/gopa/core/util"
	"github.com/infinitbyte/gopa/modules/config"
	"net/http"
	"time"
)

const Fetch JointKey = "fetch"
const Proxy ParaKey = "proxy"
const Cookie ParaKey = "cookie"

type FetchJoint struct {
	Parameters
	timeout time.Duration
}

func (this FetchJoint) Name() string {
	return string(Fetch)
}

type signal struct {
	flag   bool
	err    error
	status model.TaskStatus
}

func (this FetchJoint) Process(context *Context) error {

	this.timeout = 10 * time.Second
	timer := time.NewTimer(this.timeout)
	defer timer.Stop()

	task := context.MustGet(CONTEXT_CRAWLER_TASK).(*model.Task)
	snapshot := context.MustGet(CONTEXT_CRAWLER_SNAPSHOT).(*model.Snapshot)

	requestUrl := task.Url

	if len(requestUrl) == 0 {
		log.Error("invalid fetchUrl,", requestUrl)
		context.ErrorExit("invalid fetch url")
		return errors.New("invalid fetchUrl")
	}

	t1 := time.Now().UTC()
	task.LastFetchTime = &t1

	log.Debug("start fetch url,", requestUrl)
	flg := make(chan signal, 1)
	go func() {

		cookie, _ := this.GetString(Cookie)
		proxy, _ := this.GetString(Proxy) //"socks5://127.0.0.1:9150"  //TODO 这个是全局配置,后续的url应该也使用同样的配置,应该在domain setting里面

		//先全局,再domain,再task,再pipeline,层层覆盖
		log.Trace("proxy:", proxy)

		//start to fetch remote content
		result, err := util.HttpGetWithCookie(requestUrl, cookie, proxy)

		if err == nil && result != nil {

			task.Url = result.Url //update url, in case catch redirects
			task.Host = result.Host

			snapshot.Payload = result.Body
			snapshot.StatusCode = result.StatusCode
			snapshot.Size = result.Size
			snapshot.Headers = result.Headers

			if result.Body != nil {

				if snapshot.StatusCode == 404 {
					log.Info("skip while 404, ", requestUrl, " , ", snapshot.StatusCode)
					context.Break("fetch 404")
					flg <- signal{flag: false, err: errors.New("404 NOT FOUND"), status: model.Task404Ignore}
					return
				}

				//detect content-type
				if snapshot.Headers != nil && len(snapshot.Headers) > 0 {
					v, ok := snapshot.Headers["content-type"]
					if ok {
						if len(v) > 0 {
							s := v[0]
							if s != "" {
								snapshot.ContentType = s
							} else {
								n := 512 // Only the first 512 bytes are used to sniff the content type.
								buffer := make([]byte, n)
								if len(snapshot.Payload) < n {
									n = len(snapshot.Payload)
								}
								// Always returns a valid content-type and "application/octet-stream" if no others seemed to match.
								contentType := http.DetectContentType(buffer[:n])
								snapshot.ContentType = contentType
							}
						}

					}

				}

			}
			log.Debug("exit fetchUrl method:", requestUrl)
			flg <- signal{flag: true, status: model.TaskFetchSuccess}

		} else {

			code, payload := errors.CodeWithPayload(err)

			if code == errors.URLRedirected {
				log.Trace(util.ToJson(context, true))
				task := model.NewTaskSeed(payload.(string), requestUrl, task.Depth, task.Breadth)
				log.Trace(err)
				queue.Push(config.CheckChannel, task.MustGetBytes())
				flg <- signal{flag: false, err: err, status: model.TaskRedirectedIgnore}
				return
			}

			flg <- signal{flag: false, err: err, status: model.TaskFetchFailed}
		}
	}()

	//监听通道，由于设有超时，不可能泄露
	select {
	case <-timer.C:
		log.Error("fetching url time out, ", requestUrl, ", ", this.timeout)
		stats.Increment("domain.stats", task.Host+"."+config.STATS_FETCH_TIMEOUT_COUNT)
		task.Status = model.TaskFetchTimeout
		context.Break(fmt.Sprintf("fetching url time out, %s, %s", requestUrl, this.timeout))
		return errors.New("fetch url time out")
	case value := <-flg:
		if value.flag {
			log.Debug("fetching url normal exit, ", requestUrl)
			stats.Increment("domain.stats", task.Host+"."+config.STATS_FETCH_SUCCESS_COUNT)
		} else {
			log.Debug("fetching url error exit, ", requestUrl)
			if value.err != nil {
				context.Break(value.err.Error())
			}
			stats.Increment("domain.stats", task.Host+"."+config.STATS_FETCH_FAIL_COUNT)
		}
		task.Status = value.status
		return nil
	}

	return nil
}
