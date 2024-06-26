// Copyright (c) 2024 Baidu, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package appbuilder

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

type AppBuilderClientRawResponse struct {
	RequestID      string           `json:"request_id"`
	Date           string           `json:"date"`
	Answer         string           `json:"answer"`
	ConversationID string           `json:"conversation_id"`
	MessageID      string           `json:"message_id"`
	IsCompletion   bool             `json:"is_completion"`
	Content        []RawEventDetail `json:"content"`
}

type AppBuilderClientAnswer struct {
	Answer string
	Events []Event
}

func (t *AppBuilderClientAnswer) transform(inp *AppBuilderClientRawResponse) {
	t.Answer = inp.Answer
	for _, c := range inp.Content {
		ev := Event{Code: c.EventCode,
			Message:     c.EventMessage,
			Status:      c.EventStatus,
			EventType:   c.EventType,
			ContentType: c.ContentType,
			Detail:      c.Outputs}
		tp, ok := TypeToStruct[ev.ContentType]
		if !ok {
			tp = reflect.TypeOf(DefaultDetail{})
		}
		v := reflect.New(tp)
		_ = json.Unmarshal(c.Outputs, v.Interface())
		ev.Detail = v.Elem().Interface()
		t.Events = append(t.Events, ev)
	}
}

// AppBuilderClientIterator 定义AppBuilderClient流式/非流式迭代器接口
// 初始状态可迭代,如果返回error不为空则代表迭代结束，
// error为io.EOF，则代表迭代正常结束，其它则为异常结束
type AppBuilderClientIterator interface {
	// Next 获取处理结果，如果返回error不为空，迭代器自动失效，不允许再调用此方法
	Next() (*AppBuilderClientAnswer, error)
}

type AppBuilderClientStreamIterator struct {
	requestID string
	r         *sseReader
	body      io.ReadCloser
}

func (t *AppBuilderClientStreamIterator) Next() (*AppBuilderClientAnswer, error) {
	data, err := t.r.ReadMessageLine()
	if err != nil && !errors.Is(err, io.EOF) {
		t.body.Close()
		return nil, fmt.Errorf("requestID=%s, err=%v", t.requestID, err)
	}
	if err != nil && errors.Is(err, io.EOF) {
		t.body.Close()
		return nil, err
	}
	if strings.HasPrefix(string(data), "data:") {
		var resp AppBuilderClientRawResponse
		if err := json.Unmarshal(data[5:], &resp); err != nil {
			t.body.Close()
			return nil, fmt.Errorf("requestID=%s, err=%v", t.requestID, err)
		}
		answer := &AppBuilderClientAnswer{}
		answer.transform(&resp)
		return answer, nil
	}
	// 非SSE格式关闭连接，并返回数据
	t.body.Close()
	return nil, fmt.Errorf("requestID=%s, body=%s", t.requestID, string(data))
}

// AppBuilderClientOnceIterator 非流式返回时对应的迭代器，只可迭代一次
type AppBuilderClientOnceIterator struct {
	body      io.ReadCloser
	requestID string
}

func (t *AppBuilderClientOnceIterator) Next() (*AppBuilderClientAnswer, error) {
	data, err := io.ReadAll(t.body)
	if err != nil {
		return nil, fmt.Errorf("requestID=%s, err=%v", t.requestID, err)
	}
	defer t.body.Close()
	var resp AppBuilderClientRawResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("requestID=%s, err=%v", t.requestID, err)
	}
	answer := &AppBuilderClientAnswer{}
	answer.transform(&resp)
	return answer, nil
}
