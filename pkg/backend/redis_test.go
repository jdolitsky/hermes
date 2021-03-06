package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/codefresh-io/hermes/pkg/codefresh"
	"github.com/codefresh-io/hermes/pkg/model"
	"github.com/codefresh-io/hermes/pkg/provider"
	"github.com/codefresh-io/hermes/pkg/util"
	"github.com/garyburd/redigo/redis"
	"github.com/rafaeljusto/redigomock"
	"github.com/stretchr/testify/mock"
)

func setContext(account string) context.Context {
	return context.WithValue(context.Background(), model.ContextKeyAccount, account)
}

type RedisPoolMock struct {
	conn *redigomock.Conn
}

func (r *RedisPoolMock) GetConn() redis.Conn {
	if r.conn == nil {
		r.conn = redigomock.NewConn()
	}
	return r.conn
}

func Test_getTriggerKey(t *testing.T) {
	type args struct {
		account string
		id      string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "without prefix",
			args: args{
				account: "test-account",
				id:      "github.com:project:test",
			},
			want: "trigger:github.com:project:test:" + model.CalculateAccountHash("test-account"),
		},
		{
			name: "without prefix public",
			args: args{
				account: model.PublicAccount,
				id:      "github.com:project:test",
			},
			want: "trigger:github.com:project:test:" + model.PublicAccountHash,
		},
		{
			name: "with prefix",
			args: args{
				account: "test-account",
				id:      "trigger:github.com:project:test",
			},
			want: "trigger:github.com:project:test:" + model.CalculateAccountHash("test-account"),
		},
		{
			name: "with prefix and suffix",
			args: args{
				account: "test-account",
				id:      "trigger:github.com:project:test:" + model.CalculateAccountHash("test-account"),
			},
			want: "trigger:github.com:project:test:" + model.CalculateAccountHash("test-account"),
		},
		{
			name: "empty",
			args: args{
				account: "test-account",
				id:      "",
			},
			want: "trigger:*:" + model.CalculateAccountHash("test-account"),
		},
		{
			name: "any account",
			args: args{
				account: "-",
				id:      "not:changing:id",
			},
			want: "trigger:not:changing:id",
		},
		{
			name: "star",
			args: args{
				account: "test-account",
				id:      "*",
			},
			want: "trigger:*:" + model.CalculateAccountHash("test-account"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getTriggerKey(tt.args.account, tt.args.id); got != tt.want {
				t.Errorf("getKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMain(m *testing.M) {
	util.TestMode = true
	os.Exit(m.Run())
}

func TestRedisStore_Ping(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		wantErr bool
	}{
		{
			"happy ping",
			"PONG",
			false,
		},
		{
			"failed ping - no Redis",
			"",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RedisStore{
				redisPool: &RedisPoolMock{},
			}
			if tt.wantErr {
				r.redisPool.GetConn().(*redigomock.Conn).Command("PING").ExpectError(fmt.Errorf("PING error"))
			} else {
				r.redisPool.GetConn().(*redigomock.Conn).Command("PING").Expect(tt.want)
			}
			got, err := r.Ping()
			if (err != nil) != tt.wantErr {
				t.Errorf("RedisStore.Ping() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("RedisStore.Ping() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisStore_GetTriggerPipelines(t *testing.T) {
	type args struct {
		account string
		event   string
	}
	tests := []struct {
		name      string
		args      args
		pipelines []string
		redisErr  error
		wantErr   error
	}{
		{
			name: "get pipelines for event",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
			},
			pipelines: []string{"pipeline1", "pipeline2", "pipeline3"},
		},
		{
			name: "no pipelines for event",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
			},
			wantErr: model.ErrPipelineNotFound,
		},
		{
			name: "redis ZRANGE ErrNil error",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
			},
			redisErr: redis.ErrNil,
			wantErr:  model.ErrTriggerNotFound,
		},
		{
			name: "redis ZRANGE error",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
			},
			redisErr: redis.ErrPoolExhausted,
			wantErr:  redis.ErrPoolExhausted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RedisStore{
				redisPool: &RedisPoolMock{},
			}
			cmd := r.redisPool.GetConn().(*redigomock.Conn).Command("ZRANGE", getTriggerKey(tt.args.account, tt.args.event), 0, -1)
			if tt.redisErr != nil {
				cmd.ExpectError(tt.redisErr)
			} else {
				cmd.Expect(util.InterfaceSlice(tt.pipelines))
			}

			got, err := r.GetTriggerPipelines(setContext(tt.args.account), tt.args.event)
			if err != nil && err.Error() != tt.wantErr.Error() {
				t.Errorf("RedisStore.GetPipelinesForTriggers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (len(got) > 0 || len(tt.pipelines) > 0) && !reflect.DeepEqual(got, tt.pipelines) {
				t.Errorf("RedisStore.GetPipelinesForTriggers() = %v, want %v", got, tt.pipelines)
			}
		})
	}
}

func TestRedisStore_DeleteTrigger(t *testing.T) {
	type Errors struct {
		mismatch         bool
		nonexisting      bool
		pipelinemismatch bool
		multi            bool
		zrem1            bool
		zrem2            bool
		exec             bool
	}
	type args struct {
		account  string
		event    string
		pipeline string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		errs    Errors
	}{
		{
			name: "delete trigger: private event <-> pipeline",
			args: args{
				account:  model.PublicAccount,
				event:    "uri:test:" + model.PublicAccountHash,
				pipeline: "owner:repo:test",
			},
		},
		{
			name: "delete trigger: public event <-> pipeline",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.PublicAccountHash,
				pipeline: "owner:repo:test",
			},
		},
		{
			name: "account mismatch",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("B"),
				pipeline: "owner:repo:test",
			},
			errs:    Errors{mismatch: true},
			wantErr: true,
		},
		{
			name: "pipeline account mismatch",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			errs:    Errors{pipelinemismatch: true},
			wantErr: true,
		},
		{
			name: "non-existing pipeline",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			errs:    Errors{nonexisting: true},
			wantErr: true,
		},
		{
			name: "fail start transaction",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			wantErr: true,
			errs:    Errors{multi: true},
		},
		{
			name: "fail deleting pipeline from Triggers",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			wantErr: true,
			errs:    Errors{zrem1: true},
		},
		{
			name: "fail deleting event from Pipelines",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			wantErr: true,
			errs:    Errors{zrem2: true},
		},
		{
			name: "fail exec transaction",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			wantErr: true,
			errs:    Errors{exec: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := codefresh.NewCodefreshMockEndpoint()
			r := &RedisStore{
				redisPool:   &RedisPoolMock{},
				pipelineSvc: mock,
			}
			var cmd *redigomock.Cmd
			// check mismatch account
			if tt.errs.mismatch {
				goto Invoke
			}
			// mock Codefresh API call
			if tt.errs.nonexisting {
				mock.On("GetPipeline", tt.args.account, tt.args.pipeline).Return(nil, codefresh.ErrPipelineNotFound)
				goto Invoke
			} else if tt.errs.pipelinemismatch {
				mock.On("GetPipeline", tt.args.account, tt.args.pipeline).Return(nil, codefresh.ErrPipelineNoMatch)
				goto Invoke
			} else {
				mock.On("GetPipeline", tt.args.account, tt.args.pipeline).Return(&codefresh.Pipeline{
					ID:      tt.args.pipeline,
					Account: tt.args.account,
				}, nil)
			}
			// expect Redis transaction open
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("MULTI")
			if tt.errs.multi {
				cmd.ExpectError(errors.New("MULTI error"))
				goto Invoke
			} else {
				cmd.Expect("OK!")
			}
			// remove pipeline from Triggers
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("ZREM", getTriggerKey(tt.args.account, tt.args.event), tt.args.pipeline)
			if tt.errs.zrem1 {
				cmd.ExpectError(errors.New("ZREM error"))
				goto EndTransaction
			}

			// remove event from Pipelines
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("ZREM", getPipelineKey(tt.args.pipeline), tt.args.event)
			if tt.errs.zrem2 {
				cmd.ExpectError(errors.New("ZREM error"))
			}

		EndTransaction:
			// discard transaction on error
			if tt.wantErr && !tt.errs.exec {
				// expect transaction discard on error
				r.redisPool.GetConn().(*redigomock.Conn).Command("DISCARD").Expect("OK!")
			} else {
				// expect Redis transaction exec
				cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("EXEC")
				if tt.errs.exec {
					cmd.ExpectError(errors.New("EXEC error"))
				} else {
					cmd.Expect("OK!")
				}
			}

			// invoke method
		Invoke:
			if err := r.DeleteTrigger(setContext(tt.args.account), tt.args.event, tt.args.pipeline); (err != nil) != tt.wantErr {
				t.Errorf("RedisStore.DeleteTriggersForPipeline() error = %v, wantErr %v", err, tt.wantErr)
			}
			// assert mock
			mock.AssertExpectations(t)
		})
	}
}

func TestRedisStore_CreateTrigger(t *testing.T) {
	type Errors struct {
		mismatch         bool
		nonexisting      bool
		pipelinemismatch bool
		multi            bool
		zadd1            bool
		zadd2            bool
		exec             bool
	}
	type args struct {
		account  string
		event    string
		pipeline string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
		errs    Errors
	}{
		{
			name: "create trigger: private event <-> pipeline",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
		},
		{
			name: "create trigger: public event <-> pipeline",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.PublicAccountHash,
				pipeline: "owner:repo:test",
			},
		},
		{
			name: "account mismatch",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("B"),
				pipeline: "owner:repo:test",
			},
			errs:    Errors{mismatch: true},
			wantErr: true,
		},
		{
			name: "pipeline account mismatch",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			errs:    Errors{pipelinemismatch: true},
			wantErr: true,
		},
		{
			name: "non-existing pipeline",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			errs:    Errors{nonexisting: true},
			wantErr: true,
		},
		{
			name: "fail start transaction",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			wantErr: true,
			errs:    Errors{multi: true},
		},
		{
			name: "fail adding pipeline to Triggers",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			wantErr: true,
			errs:    Errors{zadd1: true},
		},
		{
			name: "fail adding event to Pipelines",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			wantErr: true,
			errs:    Errors{zadd2: true},
		},
		{
			name: "fail exec transaction",
			args: args{
				account:  "A",
				event:    "uri:test:" + model.CalculateAccountHash("A"),
				pipeline: "owner:repo:test",
			},
			wantErr: true,
			errs:    Errors{exec: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := codefresh.NewCodefreshMockEndpoint()
			r := &RedisStore{
				redisPool:   &RedisPoolMock{},
				pipelineSvc: mock,
			}
			var cmd *redigomock.Cmd
			// check mismatch account
			if tt.errs.mismatch {
				goto Invoke
			}
			// mock Codefresh API call
			if tt.errs.nonexisting {
				mock.On("GetPipeline", tt.args.account, tt.args.pipeline).Return(nil, codefresh.ErrPipelineNotFound)
				goto Invoke
			} else if tt.errs.pipelinemismatch {
				mock.On("GetPipeline", tt.args.account, tt.args.pipeline).Return(nil, codefresh.ErrPipelineNoMatch)
				goto Invoke
			} else {
				mock.On("GetPipeline", tt.args.account, tt.args.pipeline).Return(&codefresh.Pipeline{
					ID:      tt.args.pipeline,
					Account: tt.args.account,
				}, nil)
			}
			// expect Redis transaction open
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("MULTI")
			if tt.errs.multi {
				cmd.ExpectError(errors.New("MULTI error"))
				goto Invoke
			} else {
				cmd.Expect("OK!")
			}
			// add event to the Pipelines set
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("ZADD", getPipelineKey(tt.args.pipeline), 0, tt.args.event)
			if tt.errs.zadd1 {
				cmd.ExpectError(errors.New("ZADD error"))
				goto EndTransaction
			}

			// add pipeline to the Triggers map
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("ZADD", getTriggerKey(tt.args.account, tt.args.event), 0, tt.args.pipeline)
			if tt.errs.zadd2 {
				cmd.ExpectError(errors.New("ZADD error"))
			}

		EndTransaction:
			// discard transaction on error
			if tt.wantErr && !tt.errs.exec {
				// expect transaction discard on error
				r.redisPool.GetConn().(*redigomock.Conn).Command("DISCARD").Expect("OK!")
			} else {
				// expect Redis transaction exec
				cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("EXEC")
				if tt.errs.exec {
					cmd.ExpectError(errors.New("EXEC error"))
				} else {
					cmd.Expect("OK!")
				}
			}

		Invoke:
			if err := r.CreateTrigger(setContext(tt.args.account), tt.args.event, tt.args.pipeline); (err != nil) != tt.wantErr {
				t.Errorf("RedisStore.CreateTriggersForEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
			// assert mock
			mock.AssertExpectations(t)
		})
	}
}

func TestRedisStore_GetEventTriggers(t *testing.T) {
	type redisErrors struct {
		keys   bool
		zrange bool
	}
	type args struct {
		event   string
		account string
	}
	type triggers struct {
		event     string
		pipelines []string
	}
	tests := []struct {
		name            string
		args            args
		privateTriggers []triggers
		publicTriggers  []triggers
		want            []model.Trigger
		errs            redisErrors
		wantErr         bool
	}{
		{
			name: "list triggers for public event",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
			},
			publicTriggers: []triggers{
				{
					event:     "uri:test:" + model.PublicAccountHash,
					pipelines: []string{"pipeline-1", "pipeline-2"},
				},
			},
			want: []model.Trigger{
				model.Trigger{Event: "uri:test:" + model.PublicAccountHash, Pipeline: "pipeline-1"},
				model.Trigger{Event: "uri:test:" + model.PublicAccountHash, Pipeline: "pipeline-2"},
			},
			errs: redisErrors{false, false},
		},
		{
			name: "list triggers for account event",
			args: args{
				account: "test-account",
				event:   "uri:test:bcd5ffa2db6e",
			},
			privateTriggers: []triggers{
				{
					event:     "uri:test:bcd5ffa2db6e",
					pipelines: []string{"pipeline-1", "pipeline-2"},
				},
			},
			want: []model.Trigger{
				model.Trigger{Event: "uri:test:bcd5ffa2db6e", Pipeline: "pipeline-1"},
				model.Trigger{Event: "uri:test:bcd5ffa2db6e", Pipeline: "pipeline-2"},
			},
			errs: redisErrors{false, false},
		},
		{
			name: "list triggers for multiple public events",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:*",
			},
			publicTriggers: []triggers{
				{
					event:     "uri:test:1:" + model.PublicAccountHash,
					pipelines: []string{"pipeline-1", "pipeline-2"},
				},
				{
					event:     "uri:test:2:" + model.PublicAccountHash,
					pipelines: []string{"pipeline-2", "pipeline-3"},
				},
			},
			want: []model.Trigger{
				model.Trigger{Event: "uri:test:1:" + model.PublicAccountHash, Pipeline: "pipeline-1"},
				model.Trigger{Event: "uri:test:1:" + model.PublicAccountHash, Pipeline: "pipeline-2"},
				model.Trigger{Event: "uri:test:2:" + model.PublicAccountHash, Pipeline: "pipeline-2"},
				model.Trigger{Event: "uri:test:2:" + model.PublicAccountHash, Pipeline: "pipeline-3"},
			},
			errs: redisErrors{false, false},
		},
		{
			name: "list triggers for multiple private and public events",
			args: args{
				account: "A",
				event:   "uri:test:*",
			},
			privateTriggers: []triggers{
				{
					event:     "uri:test:1:" + model.CalculateAccountHash("A"),
					pipelines: []string{"pipeline-1", "pipeline-2"},
				},
			},
			publicTriggers: []triggers{
				{
					event:     "uri:test:2:" + model.PublicAccountHash,
					pipelines: []string{"pipeline-2", "pipeline-3"},
				},
			},
			want: []model.Trigger{
				model.Trigger{Event: "uri:test:1:" + model.CalculateAccountHash("A"), Pipeline: "pipeline-1"},
				model.Trigger{Event: "uri:test:1:" + model.CalculateAccountHash("A"), Pipeline: "pipeline-2"},
				model.Trigger{Event: "uri:test:2:" + model.PublicAccountHash, Pipeline: "pipeline-2"},
				model.Trigger{Event: "uri:test:2:" + model.PublicAccountHash, Pipeline: "pipeline-3"},
			},
			errs: redisErrors{false, false},
		},
		{
			name: "fail to find trigger by event",
			args: args{
				event: "non-existing-event",
			},
			errs:    redisErrors{true, false},
			wantErr: true,
		},
		{
			name: "fail to find pipelines for event",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
			},
			publicTriggers: []triggers{
				{
					event: "uri:test:" + model.PublicAccountHash,
				},
				{
					event:     "uri:test:other",
					pipelines: []string{"pipeline-2", "pipeline-3"},
				},
			},
			errs:    redisErrors{false, true},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RedisStore{
				redisPool: &RedisPoolMock{},
			}
			// merge all triggers
			triggers := make([]triggers, 0)
			triggers = append(triggers, tt.privateTriggers...)
			triggers = append(triggers, tt.publicTriggers...)
			// get keys from Triggers Set
			keys := make([]string, 0)
			cmd := r.redisPool.GetConn().(*redigomock.Conn).Command("KEYS", getTriggerKey(tt.args.account, tt.args.event))
			if tt.errs.keys {
				cmd.ExpectError(errors.New("KEYS error"))
				goto Invoke
			} else {
				for _, t := range tt.privateTriggers {
					keys = util.MergeStrings(keys, []string{getTriggerKey(tt.args.account, t.event)})
				}
				cmd.Expect(util.InterfaceSlice(keys))
			}
			// get public triggers matching event
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("KEYS", getTriggerKey(model.PublicAccount, tt.args.event))
			if tt.errs.keys {
				cmd.ExpectError(errors.New("KEYS error"))
				goto Invoke
			} else {
				for _, t := range tt.publicTriggers {
					keys = util.MergeStrings(keys, []string{getTriggerKey(model.PublicAccount, t.event)})
				}
				cmd.Expect(util.InterfaceSlice(keys))
			}

			// get pipelines from Triggers Set
			for _, k := range keys {
				cmd := r.redisPool.GetConn().(*redigomock.Conn).Command("ZRANGE", k, 0, -1)
				if tt.errs.zrange {
					cmd.ExpectError(errors.New("ZRANGE error"))
					goto Invoke
				} else {
					for _, t := range triggers {
						// select pipelines for key
						if getTriggerKey(tt.args.account, t.event) == k {
							cmd.Expect(util.InterfaceSlice(t.pipelines))
							break
						}
					}
				}
			}
		Invoke:
			got, err := r.GetEventTriggers(setContext(tt.args.account), tt.args.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("RedisStore.GetEventTriggers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RedisStore.GetEventTriggers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisStore_GetPipelineTriggers(t *testing.T) {
	type redisErrors struct {
		exists bool
		zrange bool
	}
	type pipeline struct {
		events []string
	}
	type args struct {
		account  string
		pipeline string
	}
	tests := []struct {
		name     string
		args     args
		pipeline pipeline
		want     []model.Trigger
		errs     redisErrors
		wantErr  bool
	}{
		{
			name: "list public triggers for pipeline",
			args: args{
				account:  model.PublicAccount,
				pipeline: "test-pipeline",
			},
			pipeline: pipeline{
				events: []string{
					"event:1:" + model.PublicAccountHash,
					"event:2:" + model.PublicAccountHash,
				},
			},
			want: []model.Trigger{
				model.Trigger{Event: "event:1:" + model.PublicAccountHash, Pipeline: "test-pipeline"},
				model.Trigger{Event: "event:2:" + model.PublicAccountHash, Pipeline: "test-pipeline"},
			},
			errs:    redisErrors{false, false},
			wantErr: false,
		},
		{
			name: "list public and private triggers for pipeline",
			args: args{
				account:  "A",
				pipeline: "test-pipeline",
			},
			pipeline: pipeline{
				events: []string{
					"event:1:" + model.PublicAccountHash,
					"event:2:" + model.PublicAccountHash,
					"event:3:" + model.CalculateAccountHash("A"),
					"event:4:" + model.CalculateAccountHash("B"),
					"event:5:" + model.CalculateAccountHash("A"),
				},
			},
			want: []model.Trigger{
				model.Trigger{Event: "event:1:" + model.PublicAccountHash, Pipeline: "test-pipeline"},
				model.Trigger{Event: "event:2:" + model.PublicAccountHash, Pipeline: "test-pipeline"},
				model.Trigger{Event: "event:3:" + model.CalculateAccountHash("A"), Pipeline: "test-pipeline"},
				model.Trigger{Event: "event:5:" + model.CalculateAccountHash("A"), Pipeline: "test-pipeline"},
			},
			errs:    redisErrors{false, false},
			wantErr: false,
		},
		{
			name: "get triggers for non-existing pipeline",
			args: args{
				pipeline: "non-existing-pipeline",
			},
			errs:    redisErrors{exists: true},
			wantErr: true,
		},
		{
			name: "fail to get another account triggers for pipeline",
			args: args{
				account:  "A",
				pipeline: "test-pipeline",
			},
			pipeline: pipeline{
				events: []string{
					"event:1:" + model.CalculateAccountHash("B"),
					"event:2:" + model.CalculateAccountHash("C"),
				},
			},
			wantErr: true,
		},
		{
			name: "fail to get triggers REDIS ZRANGE error",
			args: args{
				account:  "A",
				pipeline: "test-pipeline",
			},
			errs:    redisErrors{zrange: true},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RedisStore{
				redisPool: &RedisPoolMock{},
			}
			// get keys from Pipelines Set
			pipelineKey := getPipelineKey(tt.args.pipeline)
			cmd := r.redisPool.GetConn().(*redigomock.Conn).Command("EXISTS", pipelineKey)
			if tt.errs.exists {
				cmd.ExpectError(errors.New("EXISTS error"))
				goto Invoke
			} else {
				cmd.Expect(int64(1))
			}

			// get events from Pipeline Set
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("ZRANGE", pipelineKey, 0, -1)
			if tt.errs.zrange {
				cmd.ExpectError(errors.New("ZRANGE error"))
				goto Invoke
			} else {
				cmd.Expect(util.InterfaceSlice(tt.pipeline.events))
			}

		Invoke:
			got, err := r.GetPipelineTriggers(setContext(tt.args.account), tt.args.pipeline)
			if (err != nil) != tt.wantErr {
				t.Errorf("RedisStore.GetPipelineTriggers() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RedisStore.GetPipelineTriggers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisStore_GetEvent(t *testing.T) {
	type args struct {
		event   string
		account string
	}
	type expect struct {
		account string
		fields  map[string]string
	}
	tests := []struct {
		name           string
		args           args
		expect         expect
		want           *model.Event
		anotherAccount bool
		notExists      bool
		wantErr        error
		keyErr         bool
	}{
		{
			name: "get existing event",
			args: args{account: model.PublicAccount, event: "uri:test:" + model.PublicAccountHash},
			expect: expect{
				fields: map[string]string{
					"type":        "test-type",
					"kind":        "test-kind",
					"account":     model.PublicAccount,
					"secret":      "test-secret",
					"endpoint":    "http://endpoint",
					"description": "test-desc",
					"status":      "test",
					"help":        "test-help",
				},
			},
			want: &model.Event{
				URI:     "uri:test:" + model.PublicAccountHash,
				Type:    "test-type",
				Account: model.PublicAccount,
				Kind:    "test-kind",
				Secret:  "test-secret",
				EventInfo: model.EventInfo{
					Endpoint:    "http://endpoint",
					Description: "test-desc",
					Status:      "test",
					Help:        "test-help",
				},
			},
		},
		{
			name: "get existing private event",
			args: args{event: "uri:test:" + model.CalculateAccountHash("test-account")},
			expect: expect{
				account: "test-account",
				fields: map[string]string{
					"type":        "test-type",
					"kind":        "test-kind",
					"account":     "test-account",
					"secret":      "test-secret",
					"endpoint":    "http://endpoint",
					"description": "test-desc",
					"status":      "test",
					"help":        "test-help",
				},
			},
			want: &model.Event{
				URI:     "uri:test:" + model.CalculateAccountHash("test-account"),
				Type:    "test-type",
				Kind:    "test-kind",
				Account: "test-account",
				Secret:  "test-secret",
				EventInfo: model.EventInfo{
					Endpoint:    "http://endpoint",
					Description: "test-desc",
					Status:      "test",
					Help:        "test-help",
				},
			},
		},
		{
			name:           "get trigger event from another account",
			args:           args{event: "event:uri:test", account: "test-account"},
			expect:         expect{account: "another"},
			anotherAccount: true,
			wantErr:        model.ErrEventNotFound,
		},
		{
			name:      "get non-existing event",
			args:      args{event: "non-existing:event:uri:test"},
			expect:    expect{},
			notExists: true,
			wantErr:   model.ErrEventNotFound,
		},
		{
			name:    "get event REDIS error",
			args:    args{event: "uri:test"},
			expect:  expect{},
			wantErr: errors.New("EXISTS error"),
			keyErr:  true,
		},
		{
			name:    "get non-existing event REDIS error",
			args:    args{event: "non-existing:event:uri:test"},
			expect:  expect{},
			wantErr: errors.New("HGETALL error"),
		},
		{
			name:      "try getting event with invalid key",
			args:      args{event: "uri:*"},
			expect:    expect{},
			notExists: true,
			wantErr:   model.ErrEventNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RedisStore{
				redisPool: &RedisPoolMock{},
			}
			eventKey := getEventKey(tt.args.account, tt.args.event)
			cmd := r.redisPool.GetConn().(*redigomock.Conn).Command("EXISTS", eventKey)
			if tt.keyErr {
				cmd.ExpectError(tt.wantErr)
				goto Invoke
			}
			if tt.notExists {
				cmd.Expect(int64(0))
				goto Invoke
			} else {
				cmd.Expect(int64(1))
			}
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HGETALL", eventKey)
			if tt.wantErr != nil && tt.wantErr != model.ErrEventNotFound {
				cmd.ExpectError(tt.wantErr)
			} else {
				cmd.ExpectMap(tt.expect.fields)
			}
		Invoke:
			got, err := r.GetEvent(setContext(tt.args.account), tt.args.event)
			if err != tt.wantErr {
				t.Errorf("RedisStore.GetEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RedisStore.GetEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisStore_GetEvents(t *testing.T) {
	type Errors struct {
		keys       bool
		pubKeys    bool
		pubKeysNil bool
		exists     bool
		hgetall    bool
	}
	type expect struct {
		keys    []string
		pubKeys []string
		fields  []map[string]string
	}
	type args struct {
		eventType string
		kind      string
		account   string
		filter    string
		public    bool
	}
	tests := []struct {
		name    string
		args    args
		expect  expect
		want    []model.Event
		errs    Errors
		wantErr bool
	}{
		{
			name: "get all trigger events",
			args: args{account: "A"},
			expect: expect{
				keys: []string{
					"uri:1:" + model.CalculateAccountHash("A"),
					"uri:2:" + model.CalculateAccountHash("A"),
					"uri:3:" + model.CalculateAccountHash("A"),
				},
				fields: []map[string]string{
					{"type": "t1", "kind": "k1", "secret": "s1", "account": "A"},
					{"type": "t2", "kind": "k2", "secret": "s2", "account": "A"},
					{"type": "t3", "kind": "k3", "secret": "s3", "account": "A"},
				},
			},
			want: []model.Event{
				{URI: "uri:1:" + model.CalculateAccountHash("A"), Type: "t1", Kind: "k1", Secret: "s1", Account: "A"},
				{URI: "uri:2:" + model.CalculateAccountHash("A"), Type: "t2", Kind: "k2", Secret: "s2", Account: "A"},
				{URI: "uri:3:" + model.CalculateAccountHash("A"), Type: "t3", Kind: "k3", Secret: "s3", Account: "A"},
			},
		},
		{
			name: "get trigger events by type",
			args: args{eventType: "T"},
			expect: expect{
				keys: []string{"uri:1", "uri:2", "uri:3"},
				fields: []map[string]string{
					{"type": "t1", "kind": "k1", "secret": "s1"},
					{"type": "T", "kind": "k2", "secret": "s2"},
					{"type": "T", "kind": "k3", "secret": "s3"},
				},
			},
			want: []model.Event{
				{URI: "uri:2", Type: "T", Kind: "k2", Secret: "s2"},
				{URI: "uri:3", Type: "T", Kind: "k3", Secret: "s3"},
			},
		},
		{
			name: "get trigger events by account and public",
			args: args{account: "A", public: true},
			expect: expect{
				keys: []string{
					"uri:1:" + model.CalculateAccountHash("A"),
					"uri:2:" + model.CalculateAccountHash("A"),
				},
				pubKeys: []string{
					"uri:3:" + model.PublicAccountHash,
					"uri:4:" + model.PublicAccountHash,
				},
				fields: []map[string]string{
					{"type": "t1", "kind": "k1", "secret": "s1", "account": "A"},
					{"type": "t1", "kind": "k2", "secret": "s2", "account": "A"},
					{"type": "t2", "kind": "k3", "secret": "s3", "account": model.PublicAccount},
					{"type": "t3", "kind": "k2", "secret": "s4", "account": model.PublicAccount},
				},
			},
			want: []model.Event{
				{URI: "uri:1:" + model.CalculateAccountHash("A"), Type: "t1", Kind: "k1", Secret: "s1", Account: "A"},
				{URI: "uri:2:" + model.CalculateAccountHash("A"), Type: "t1", Kind: "k2", Secret: "s2", Account: "A"},
				{URI: "uri:3:" + model.PublicAccountHash, Type: "t2", Kind: "k3", Secret: "s3", Account: model.PublicAccount},
				{URI: "uri:4:" + model.PublicAccountHash, Type: "t3", Kind: "k2", Secret: "s4", Account: model.PublicAccount},
			},
		},
		{
			name: "get trigger events by account and public empty",
			args: args{account: "A", public: true},
			expect: expect{
				keys: []string{
					"uri:1:" + model.CalculateAccountHash("A"),
					"uri:2:" + model.CalculateAccountHash("A"),
				},
				fields: []map[string]string{
					{"type": "t1", "kind": "k1", "secret": "s1", "account": "A"},
					{"type": "t1", "kind": "k2", "secret": "s2", "account": "A"},
				},
			},
			want: []model.Event{
				{URI: "uri:1:" + model.CalculateAccountHash("A"), Type: "t1", Kind: "k1", Secret: "s1", Account: "A"},
				{URI: "uri:2:" + model.CalculateAccountHash("A"), Type: "t1", Kind: "k2", Secret: "s2", Account: "A"},
			},
			errs: Errors{pubKeysNil: true},
		},
		{
			name: "get trigger events by filter",
			args: args{account: "A", filter: "uri:2*"},
			expect: expect{
				keys: []string{"uri:21:" + model.CalculateAccountHash("A"), "uri:22:" + model.PublicAccountHash},
				fields: []map[string]string{
					{"type": "t2", "kind": "k2", "secret": "s2", "account": "A"},
					{"type": "t3", "kind": "k3", "secret": "s3", "account": model.PublicAccount},
				},
			},
			want: []model.Event{
				{URI: "uri:21:" + model.CalculateAccountHash("A"), Type: "t2", Kind: "k2", Secret: "s2", Account: "A"},
				{URI: "uri:22:" + model.PublicAccountHash, Type: "t3", Kind: "k3", Secret: "s3", Account: model.PublicAccount},
			},
		},
		{
			name: "get trigger events by type and kind",
			args: args{account: model.PublicAccount, eventType: "T", kind: "K"},
			expect: expect{
				keys: []string{
					"uri:1:" + model.PublicAccountHash,
					"uri:2:" + model.PublicAccountHash,
					"uri:3:" + model.PublicAccountHash,
				},
				fields: []map[string]string{
					{"type": "t1", "kind": "k1", "secret": "s1", "account": model.PublicAccount},
					{"type": "T", "kind": "K", "secret": "s2", "account": model.PublicAccount},
					{"type": "T", "kind": "k3", "secret": "s3", "account": model.PublicAccount},
				},
			},
			want: []model.Event{
				{URI: "uri:2:" + model.PublicAccountHash, Type: "T", Kind: "K", Secret: "s2", Account: model.PublicAccount},
			},
		},
		{
			name: "get no trigger events by type and kind",
			args: args{account: "test-account", eventType: "T", kind: "K"},
			expect: expect{
				keys: []string{
					"uri:1:" + model.CalculateAccountHash("test-account"),
					"uri:2:" + model.CalculateAccountHash("test-account"),
					"uri:3:" + model.CalculateAccountHash("test-account"),
				},
				fields: []map[string]string{
					{"type": "t1", "kind": "k1", "secret": "s1", "account": "test-account"},
					{"type": "t2", "kind": "k2", "secret": "s2", "account": "test-account"},
					{"type": "t3", "kind": "k3", "secret": "s3", "account": "test-account"},
				},
			},
		},
		{
			name:    "keys error",
			args:    args{},
			expect:  expect{},
			errs:    Errors{keys: true},
			wantErr: true,
		},
		{
			name:    "public keys error",
			args:    args{account: "A", public: true},
			expect:  expect{},
			errs:    Errors{pubKeys: true},
			wantErr: true,
		},
		{
			name: "public keys Nil error",
			args: args{account: "test-account", public: true},
			expect: expect{
				keys: []string{
					"uri:1:" + model.CalculateAccountHash("test-account"),
				},
				fields: []map[string]string{
					{"type": "t1", "kind": "k1", "secret": "s1", "account": "test-account"},
				},
			},
			want: []model.Event{
				{URI: "uri:1:" + model.CalculateAccountHash("test-account"), Type: "t1", Kind: "k1", Secret: "s1", Account: "test-account"},
			},
			errs:    Errors{pubKeysNil: true},
			wantErr: false,
		},
		{
			name: "exists error",
			args: args{account: "test-account"},
			expect: expect{
				keys: []string{
					"uri:1:" + model.CalculateAccountHash("test-account"),
				},
			},
			errs:    Errors{exists: true},
			wantErr: true,
		},
		{
			name: "hgetall error",
			args: args{account: "test-account"},
			expect: expect{
				keys: []string{
					"uri:1:" + model.CalculateAccountHash("test-account"),
				},
			},
			errs:    Errors{hgetall: true},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &RedisStore{
				redisPool: &RedisPoolMock{},
			}
			// keys includes both private and public keys
			keys := append(tt.expect.keys, tt.expect.pubKeys...)
			// mock getting trigger event keys
			cmd := r.redisPool.GetConn().(*redigomock.Conn).Command("KEYS", getEventKey(tt.args.account, tt.args.filter))
			if tt.errs.keys {
				cmd.ExpectError(errors.New("KEYS error"))
				goto Invoke
			} else {
				cmd.Expect(util.InterfaceSlice(tt.expect.keys))
			}
			// add public keys
			if tt.args.public {
				cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("KEYS", getEventKey(model.PublicAccount, tt.args.filter))
				if tt.errs.pubKeys {
					cmd.ExpectError(errors.New("Public KEYS error"))
					goto Invoke
				} else if tt.errs.pubKeysNil {
					cmd.ExpectError(redis.ErrNil)
				} else {
					cmd.Expect(util.InterfaceSlice(tt.expect.pubKeys))
				}
			}
			// mock scanning trough all trigger events
			for i, k := range keys {
				eventKey := getEventKey(tt.args.account, k)
				cmd := r.redisPool.GetConn().(*redigomock.Conn).Command("EXISTS", eventKey)
				if tt.errs.exists {
					cmd.Expect(int64(0))
					goto Invoke
				} else {
					cmd.Expect(int64(1))
				}
				cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HGETALL", eventKey)
				if tt.errs.hgetall {
					cmd.ExpectError(errors.New("HGETALL error"))
					goto Invoke
				} else {
					cmd.ExpectMap(tt.expect.fields[i])
				}
			}

			// invoke
		Invoke:
			ctx := setContext(tt.args.account)
			if tt.args.public {
				ctx = context.WithValue(ctx, model.ContextKeyPublic, true)
			}
			got, err := r.GetEvents(ctx, tt.args.eventType, tt.args.kind, tt.args.filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("RedisStore.GetEvents() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (len(got) != 0 || len(tt.want) != 0) && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RedisStore.GetEvents() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedisStore_DeleteEvent(t *testing.T) {
	type redisErrors struct {
		exists     bool
		hget       bool
		multi      bool
		zrange     bool
		delEvent   bool
		delTrigger bool
		exec       bool
	}
	type expected struct {
		account     string
		pipelines   []string
		credentials map[string]string
	}
	type args struct {
		event   string
		account string
		context string
	}
	tests := []struct {
		name           string
		args           args
		expected       expected
		errs           redisErrors
		notExists      bool
		anotherAccount bool
		wantErr        error
		wantEventErr   error
	}{
		{
			name: "delete existing trigger event",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
			},
			expected: expected{
				account: model.PublicAccount,
			},
		},
		{
			name: "delete existing trigger event with context",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
				context: `{"apikey": "1234567890"}`,
			},
			expected: expected{
				account:     model.PublicAccount,
				credentials: map[string]string{"apikey": "1234567890"},
			},
		},
		{
			name: "delete existing trigger event unsubscribe not implemented",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
			},
			expected: expected{
				account: model.PublicAccount,
			},
			wantEventErr: provider.ErrNotImplemented,
		},
		{
			name: "delete existing private trigger event",
			args: args{
				event:   "uri:test:" + model.CalculateAccountHash("A"),
				account: "A",
			},
			expected: expected{
				account: "A",
			},
		},
		{
			name: "error deleting existing private trigger event",
			args: args{
				event:   "uri:test:" + model.CalculateAccountHash("A"),
				account: "A",
			},
			expected: expected{
				account: "B",
			},
			anotherAccount: true,
			wantErr:        model.ErrEventNotFound,
		},
		{
			name: "try to delete existing trigger event linked to pipelines",
			args: args{
				account: model.PublicAccount,
				event:   "uri:test:" + model.PublicAccountHash,
			},
			expected: expected{
				account:   model.PublicAccount,
				pipelines: []string{"p1", "p2", "p3"},
			},
			wantErr: model.ErrEventDeleteWithTriggers,
		},
		{
			name: "try deleting event with invalid key",
			args: args{
				account: "test-account",
				event:   "bad-key",
			},
			notExists: true,
			wantErr:   model.ErrEventNotFound,
		},
		{
			name: "exists error",
			args: args{
				account: "test-account",
				event:   "uri:test",
			},
			notExists: true,
			wantErr:   errors.New("REDIS error"),
			errs:      redisErrors{exists: true},
		},
		{
			name: "hget error",
			args: args{
				account: "test-account",
				event:   "uri:test",
			},
			wantErr: errors.New("REDIS error"),
			errs:    redisErrors{hget: true},
		},
		{
			name: "zrange error",
			args: args{
				account: "test-account",
				event:   "uri:test",
			},
			expected: expected{
				account: "test-account",
			},
			wantErr: errors.New("REDIS error"),
			errs:    redisErrors{zrange: true},
		},
		{
			name:    "multi error",
			args:    args{event: "uri:test"},
			wantErr: errors.New("REDIS error"),
			errs:    redisErrors{multi: true},
		},
		{
			name:    "del event error",
			args:    args{event: "uri:test"},
			wantErr: errors.New("REDIS error"),
			errs:    redisErrors{delEvent: true},
		},
		{
			name:    "del trigger error",
			args:    args{event: "uri:test"},
			wantErr: errors.New("REDIS error"),
			errs:    redisErrors{delTrigger: true},
		},
		{
			name:    "exec error",
			args:    args{event: "uri:test"},
			wantErr: errors.New("REDIS error"),
			errs:    redisErrors{exec: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			epMock := provider.NewEventProviderMock()
			var call *mock.Call
			r := &RedisStore{
				redisPool:     &RedisPoolMock{},
				eventProvider: epMock,
			}
			// set context
			ctx := setContext(tt.args.account)
			// mock Redis
			eventKey := getEventKey(tt.args.account, tt.args.event)
			triggerKey := getTriggerKey(tt.args.account, tt.args.event)
			// expect Redis transaction open
			var cmd *redigomock.Cmd
			// check existence
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("EXISTS", eventKey)
			if tt.notExists {
				if tt.errs.exists {
					cmd.ExpectError(tt.wantErr)
				} else {
					cmd.Expect(int64(0))
				}
				goto Invoke
			} else {
				cmd.Expect(int64(1))
			}
			// get account
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HGET", eventKey, "account")
			if tt.errs.hget {
				cmd.ExpectError(tt.wantErr)
				goto Invoke
			} else {
				cmd.Expect(tt.expected.account)
			}
			if tt.anotherAccount {
				goto Invoke
			}
			// get trigger event pipelines
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("ZRANGE", triggerKey, 0, -1)
			if tt.errs.zrange {
				cmd.ExpectError(tt.wantErr)
				goto Invoke
			} else {
				cmd.Expect(util.InterfaceSlice(tt.expected.pipelines))
			}
			if len(tt.expected.pipelines) > 0 {
				goto Invoke
			}
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("MULTI")
			if tt.errs.multi {
				cmd.ExpectError(tt.wantErr)
				goto Invoke
			} else {
				cmd.Expect("OK!")
			}
			// delete event
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("DEL", eventKey)
			if tt.errs.delEvent {
				cmd.ExpectError(tt.wantErr)
				goto EndTransaction
			} else {
				cmd.Expect("QUEUED")
			}
			// delete trigger
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("DEL", triggerKey)
			if tt.errs.delTrigger {
				cmd.ExpectError(tt.wantErr)
				goto EndTransaction
			} else {
				cmd.Expect("QUEUED")
			}

		EndTransaction:
			// discard transaction on error
			if (tt.errs.delEvent || tt.errs.delTrigger) && !tt.errs.exec {
				// expect transaction discard on error
				r.redisPool.GetConn().(*redigomock.Conn).Command("DISCARD").Expect("OK!")
			} else {
				// expect Redis transaction exec
				cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("EXEC")
				if tt.errs.exec {
					cmd.ExpectError(tt.wantErr)
				} else {
					cmd.Expect("OK!")

					// mock event provider call
					call = epMock.On("UnsubscribeFromEvent", ctx, tt.args.event, tt.expected.credentials)
					if tt.wantEventErr != nil {
						call.Return(tt.wantEventErr)
						if tt.wantEventErr != provider.ErrNotImplemented {
							goto Invoke
						}
					} else {
						call.Return(nil)
					}
				}
			}

		Invoke:
			if err := r.DeleteEvent(ctx, tt.args.event, tt.args.context); err != tt.wantErr {
				t.Errorf("RedisStore.DeleteEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
			epMock.AssertExpectations(t)
		})
	}
}

func TestRedisStore_CreateEvent(t *testing.T) {
	type redisErrors struct {
		multi          bool
		hsetnxType     bool
		hsetnxKind     bool
		hsetnxAccount  bool
		hsetnxSecret   bool
		hsetnxDesc     bool
		hsetnxEndpoint bool
		hsetnxHelp     bool
		hsetnxStatus   bool
		exec           bool
	}
	type eventErrors struct {
		uri       error
		subscribe error
		info      error
	}
	type expected struct {
		eventURI    string
		info        *model.EventInfo
		credentials map[string]string
	}
	type args struct {
		eventType string
		kind      string
		secret    string
		account   string
		public    bool
		context   string
		values    map[string]string
	}
	tests := []struct {
		name         string
		args         args
		expected     expected
		want         *model.Event
		wantErr      bool
		wantEventErr eventErrors
		errs         redisErrors
	}{
		{
			name: "create public event",
			args: args{account: "A", public: true, eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.PublicAccountHash,
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			want: &model.Event{
				URI:       "type:kind:test:" + model.PublicAccountHash,
				Type:      "type",
				Kind:      "kind",
				Account:   model.PublicAccount,
				Secret:    "XXX",
				EventInfo: model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"}},
		},
		{
			name: "create private event (per account)",
			args: args{eventType: "type", kind: "kind", secret: "XXX", account: "5672d8deb6724b6e359adf62"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("5672d8deb6724b6e359adf62"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			want: &model.Event{
				URI:       "type:kind:test:" + model.CalculateAccountHash("5672d8deb6724b6e359adf62"),
				Type:      "type",
				Kind:      "kind",
				Account:   "5672d8deb6724b6e359adf62",
				Secret:    "XXX",
				EventInfo: model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"}},
		},
		{
			name: "create private event (per account) with credentials",
			args: args{eventType: "type", kind: "kind", secret: "XXX", account: "5672d8deb6724b6e359adf62", context: `{"apikey": "1234567890"}`},
			expected: expected{
				eventURI:    "type:kind:test:" + model.CalculateAccountHash("5672d8deb6724b6e359adf62"),
				info:        &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
				credentials: map[string]string{"apikey": "1234567890"},
			},
			want: &model.Event{
				URI:       "type:kind:test:" + model.CalculateAccountHash("5672d8deb6724b6e359adf62"),
				Type:      "type",
				Kind:      "kind",
				Account:   "5672d8deb6724b6e359adf62",
				Secret:    "XXX",
				EventInfo: model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"}},
		},
		{
			name:         "fail to construct URI",
			args:         args{eventType: "type", kind: "kind", secret: "XXX"},
			expected:     expected{eventURI: ""},
			wantEventErr: eventErrors{uri: errors.New("URI error")},
			errs:         redisErrors{},
		},
		{
			name:         "fail to subscribe to event",
			args:         args{eventType: "type", kind: "kind", secret: "XXX"},
			expected:     expected{eventURI: "type:kind:test"},
			wantEventErr: eventErrors{subscribe: errors.New("Subscribe error")},
			errs:         redisErrors{},
		},
		{
			name: "fail to subscribe to event (not implemented)",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			want: &model.Event{
				URI:       "type:kind:test:" + model.CalculateAccountHash("A"),
				Type:      "type",
				Kind:      "kind",
				Account:   "A",
				Secret:    "XXX",
				EventInfo: model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"}},
			wantEventErr: eventErrors{subscribe: provider.ErrNotImplemented},
			errs:         redisErrors{},
		},
		{
			name:         "fail to subscribe to event and get info fallback fails too",
			args:         args{eventType: "type", kind: "kind", secret: "XXX"},
			expected:     expected{eventURI: "type:kind:test"},
			wantEventErr: eventErrors{subscribe: provider.ErrNotImplemented, info: errors.New("Info error")},
			errs:         redisErrors{},
		},
		{
			name: "fail start transaction",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{multi: true},
		},
		{
			name: "fail update type",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{hsetnxType: true},
		},
		{
			name: "fail update kind",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{hsetnxKind: true},
		},
		{
			name: "fail update account",
			args: args{eventType: "type", kind: "kind", secret: "XXX", account: "5672d8deb6724b6e359adf62"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("5672d8deb6724b6e359adf62"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{hsetnxAccount: true},
		},
		{
			name: "fail update secret",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{hsetnxSecret: true},
		},
		{
			name: "fail update description",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{hsetnxDesc: true},
		},
		{
			name: "fail update endpoint",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{hsetnxEndpoint: true},
		},
		{
			name: "fail update help",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{hsetnxHelp: true},
		},
		{
			name: "fail update status",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{hsetnxStatus: true},
		},
		{
			name: "fail exec transaction",
			args: args{account: "A", eventType: "type", kind: "kind", secret: "XXX"},
			expected: expected{
				eventURI: "type:kind:test:" + model.CalculateAccountHash("A"),
				info:     &model.EventInfo{Endpoint: "test-endpoint", Description: "test-desc", Help: "test-help", Status: "test-status"},
			},
			wantErr: true,
			errs:    redisErrors{exec: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd *redigomock.Cmd
			mock := provider.NewEventProviderMock()
			r := &RedisStore{
				redisPool:     &RedisPoolMock{},
				eventProvider: mock,
			}
			account := tt.args.account
			if tt.args.public {
				account = model.PublicAccount
			}
			ctx := setContext(tt.args.account)
			if tt.args.public {
				ctx = context.WithValue(ctx, model.ContextKeyPublic, true)
			}
			// prepare key
			eventKey := getEventKey(tt.args.account, tt.expected.eventURI)
			// mock EventProvider calls
			call := mock.On("ConstructEventURI", tt.args.eventType, tt.args.kind, account, tt.args.values)
			if tt.wantEventErr.uri != nil {
				call.Return("", tt.wantEventErr.uri)
				goto Invoke
			} else {
				call.Return(tt.expected.eventURI, nil)
			}
			call = mock.On("SubscribeToEvent", ctx, tt.expected.eventURI, tt.args.secret, tt.expected.credentials)
			if tt.wantEventErr.subscribe != nil {
				call.Return(nil, tt.wantEventErr.subscribe)
				if tt.wantEventErr.subscribe == provider.ErrNotImplemented {
					call = mock.On("GetEventInfo", ctx, tt.expected.eventURI, tt.args.secret)
					if tt.wantEventErr.info != nil {
						call.Return(nil, tt.wantEventErr.info)
						goto Invoke
					} else {
						call.Return(tt.expected.info, nil)
					}
				} else {
					goto Invoke
				}
			} else {
				call.Return(tt.expected.info, nil)
			}
			// mock Redis
			// expect Redis transaction open
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("MULTI")
			if tt.errs.multi {
				cmd.ExpectError(errors.New("MULTI error"))
				goto Invoke
			} else {
				cmd.Expect("OK!")
			}
			// store Event type
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HSETNX", eventKey, "type", tt.args.eventType)
			if tt.errs.hsetnxType {
				cmd.ExpectError(errors.New("HSETNX error"))
				goto EndTransaction
			}
			// store Event kind
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HSETNX", eventKey, "kind", tt.args.kind)
			if tt.errs.hsetnxKind {
				cmd.ExpectError(errors.New("HSETNX error"))
				goto EndTransaction
			}
			// store Event account
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HSETNX", eventKey, "account", account)
			if tt.errs.hsetnxAccount {
				cmd.ExpectError(errors.New("HSETNX error"))
				goto EndTransaction
			}
			// store Event secret
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HSETNX", eventKey, "secret", tt.args.secret)
			if tt.errs.hsetnxSecret {
				cmd.ExpectError(errors.New("HSETNX error"))
				goto EndTransaction
			}
			// store Event description
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HSETNX", eventKey, "description", tt.expected.info.Description)
			if tt.errs.hsetnxDesc {
				cmd.ExpectError(errors.New("HSETNX error"))
				goto EndTransaction
			}
			// store Event endpoint
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HSETNX", eventKey, "endpoint", tt.expected.info.Endpoint)
			if tt.errs.hsetnxEndpoint {
				cmd.ExpectError(errors.New("HSETNX error"))
				goto EndTransaction
			}
			// store Event help
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HSETNX", eventKey, "help", tt.expected.info.Help)
			if tt.errs.hsetnxHelp {
				cmd.ExpectError(errors.New("HSETNX error"))
				goto EndTransaction
			}
			// store Event status
			cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("HSETNX", eventKey, "status", tt.expected.info.Status)
			if tt.errs.hsetnxStatus {
				cmd.ExpectError(errors.New("HSETNX error"))
				goto EndTransaction
			}

		EndTransaction:
			// discard transaction on error
			if tt.wantErr && !tt.errs.exec {
				// expect transaction discard on error
				r.redisPool.GetConn().(*redigomock.Conn).Command("DISCARD").Expect("OK!")
			} else {
				// expect Redis transaction exec
				cmd = r.redisPool.GetConn().(*redigomock.Conn).Command("EXEC")
				if tt.errs.exec {
					cmd.ExpectError(errors.New("EXEC error"))
				} else {
					cmd.Expect("OK!")
				}
			}

		Invoke:
			// invoke method under test
			got, err := r.CreateEvent(ctx, tt.args.eventType, tt.args.kind, tt.args.secret, tt.args.context, tt.args.values)
			if (err != nil) != (tt.wantErr ||
				tt.wantEventErr.info != nil ||
				(tt.wantEventErr.subscribe != nil &&
					tt.wantEventErr.subscribe != provider.ErrNotImplemented) ||
				tt.wantEventErr.uri != nil) {
				t.Errorf("RedisStore.CreateEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("RedisStore.CreateEvent() = %v, want %v", got, tt.want)
			}
			// assert mock
			mock.AssertExpectations(t)
		})
	}
}
