// Code generated by mockery v1.0.0
package model

import context "context"
import mock "github.com/stretchr/testify/mock"

// MockTriggerReaderWriter is an autogenerated mock type for the TriggerReaderWriter type
type MockTriggerReaderWriter struct {
	mock.Mock
}

// CreateTrigger provides a mock function with given fields: ctx, event, pipeline
func (_m *MockTriggerReaderWriter) CreateTrigger(ctx context.Context, event string, pipeline string) error {
	ret := _m.Called(ctx, event, pipeline)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, event, pipeline)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// DeleteTrigger provides a mock function with given fields: ctx, event, pipeline
func (_m *MockTriggerReaderWriter) DeleteTrigger(ctx context.Context, event string, pipeline string) error {
	ret := _m.Called(ctx, event, pipeline)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, string, string) error); ok {
		r0 = rf(ctx, event, pipeline)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetEventTriggers provides a mock function with given fields: ctx, event
func (_m *MockTriggerReaderWriter) GetEventTriggers(ctx context.Context, event string) ([]Trigger, error) {
	ret := _m.Called(ctx, event)

	var r0 []Trigger
	if rf, ok := ret.Get(0).(func(context.Context, string) []Trigger); ok {
		r0 = rf(ctx, event)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]Trigger)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, event)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetPipelineTriggers provides a mock function with given fields: ctx, pipeline
func (_m *MockTriggerReaderWriter) GetPipelineTriggers(ctx context.Context, pipeline string) ([]Trigger, error) {
	ret := _m.Called(ctx, pipeline)

	var r0 []Trigger
	if rf, ok := ret.Get(0).(func(context.Context, string) []Trigger); ok {
		r0 = rf(ctx, pipeline)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]Trigger)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, pipeline)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetTriggerPipelines provides a mock function with given fields: ctx, event
func (_m *MockTriggerReaderWriter) GetTriggerPipelines(ctx context.Context, event string) ([]string, error) {
	ret := _m.Called(ctx, event)

	var r0 []string
	if rf, ok := ret.Get(0).(func(context.Context, string) []string); ok {
		r0 = rf(ctx, event)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]string)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, event)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
