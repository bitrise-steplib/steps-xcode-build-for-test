// Code generated by mockery v2.10.4. DO NOT EDIT.

package mocks

import (
	fs "io/fs"

	mock "github.com/stretchr/testify/mock"
)

// FileManager is an autogenerated mock type for the FileManager type
type FileManager struct {
	mock.Mock
}

// ReadDir provides a mock function with given fields: name
func (_m *FileManager) ReadDir(name string) ([]fs.DirEntry, error) {
	ret := _m.Called(name)

	var r0 []fs.DirEntry
	if rf, ok := ret.Get(0).(func(string) []fs.DirEntry); ok {
		r0 = rf(name)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]fs.DirEntry)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// ReadFile provides a mock function with given fields: pth
func (_m *FileManager) ReadFile(pth string) ([]byte, error) {
	ret := _m.Called(pth)

	var r0 []byte
	if rf, ok := ret.Get(0).(func(string) []byte); ok {
		r0 = rf(pth)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(string) error); ok {
		r1 = rf(pth)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// WriteFile provides a mock function with given fields: filename, data, perm
func (_m *FileManager) WriteFile(filename string, data []byte, perm fs.FileMode) error {
	ret := _m.Called(filename, data, perm)

	var r0 error
	if rf, ok := ret.Get(0).(func(string, []byte, fs.FileMode) error); ok {
		r0 = rf(filename, data, perm)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
