// SPDX-License-Identifier: MIT
//
// Copyright © 2019 Kent Gibson <warthog618@gmail.com>.

// +build linux

package uapi_test

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/warthog618/gpiod/mockup"
	"github.com/warthog618/gpiod/uapi"
	"golang.org/x/sys/unix"
)

var (
	mock       *mockup.Mockup
	setupError error
)

func TestMain(m *testing.M) {
	reloadMockup()
	rc := m.Run()
	if mock != nil {
		mock.Close()
	}
	os.Exit(rc)
}

var (
	setConfigKernel = mockup.Semver{5, 5} // setLineConfig ioctl added
	infoWatchKernel = mockup.Semver{5, 7} // watchLineInfo ioctl added

	eventWaitTimeout         = 100 * time.Millisecond
	spuriousEventWaitTimeout = 300 * time.Millisecond
)

func reloadMockup() {
	if mock != nil {
		mock.Close()
	}
	mock, setupError = mockup.New([]int{4, 8}, true)
}

func requireMockup(t *testing.T) {
	t.Helper()
	if setupError != nil {
		t.Fail()
		t.Skip(setupError)
	}
}

func TestGetChipInfo(t *testing.T) {
	reloadMockup() // test assumes clean mockups
	requireMockup(t)
	for n := 0; n < mock.Chips(); n++ {
		c, err := mock.Chip(n)
		require.Nil(t, err)
		f := func(t *testing.T) {
			f, err := os.Open(c.DevPath)
			require.Nil(t, err)
			defer f.Close()
			cix := uapi.ChipInfo{
				Lines: uint32(c.Lines),
			}
			copy(cix.Name[:], c.Name)
			copy(cix.Label[:], c.Label)
			ci, err := uapi.GetChipInfo(f.Fd())
			assert.Nil(t, err)
			assert.Equal(t, cix, ci)
		}
		t.Run(c.Name, f)
	}
	// badfd
	ci, err := uapi.GetChipInfo(0)
	cix := uapi.ChipInfo{}
	assert.NotNil(t, err)
	assert.Equal(t, cix, ci)
}

func TestGetLineInfo(t *testing.T) {
	reloadMockup() // test assumes clean mockups
	requireMockup(t)
	for n := 0; n < mock.Chips(); n++ {
		c, err := mock.Chip(n)
		require.Nil(t, err)
		for l := 0; l < c.Lines; l++ {
			f := func(t *testing.T) {
				f, err := os.Open(c.DevPath)
				require.Nil(t, err)
				defer f.Close()
				xli := uapi.LineInfo{
					Offset: uint32(l),
					Flags:  0,
				}
				copy(xli.Name[:], fmt.Sprintf("%s-%d", c.Label, l))
				copy(xli.Consumer[:], "")
				li, err := uapi.GetLineInfo(f.Fd(), l)
				assert.Nil(t, err)
				assert.Equal(t, xli, li)
			}
			t.Run(fmt.Sprintf("%s-%d", c.Name, l), f)
		}
	}
	// badfd
	li, err := uapi.GetLineInfo(0, 1)
	xli := uapi.LineInfo{}
	assert.NotNil(t, err)
	assert.Equal(t, xli, li)
}

func TestGetLineEvent(t *testing.T) {
	requireMockup(t)
	patterns := []struct {
		name       string // unique name for pattern (hf/ef/offsets/xval combo)
		cnum       int
		handleFlag uapi.HandleFlag
		eventFlag  uapi.EventFlag
		offset     uint32
		err        error
	}{
		{
			"as-is",
			0,
			0,
			uapi.EventRequestBothEdges,
			2,
			nil,
		},
		{
			"atv-lo",
			1,
			uapi.HandleRequestActiveLow,
			uapi.EventRequestBothEdges,
			2,
			nil,
		},
		{
			"input",
			0,
			uapi.HandleRequestInput,
			uapi.EventRequestBothEdges,
			2,
			nil,
		},
		{
			"input pull-up",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestPullUp,
			uapi.EventRequestBothEdges,
			2,
			nil,
		},
		{
			"input pull-down",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestPullDown,
			uapi.EventRequestBothEdges,
			2,
			nil,
		},
		{
			"input bias disable",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestBiasDisable,
			uapi.EventRequestBothEdges,
			2,
			nil,
		},
		{
			"as-is pull-up",
			0,
			uapi.HandleRequestPullUp,
			uapi.EventRequestBothEdges,
			2,
			nil,
		},
		{
			"as-is pull-down",
			0,
			uapi.HandleRequestPullDown,
			uapi.EventRequestBothEdges,
			2,
			nil,
		},
		{
			"as-is bias disable",
			0,
			uapi.HandleRequestBiasDisable,
			uapi.EventRequestBothEdges,
			2,
			nil,
		},
		// expected errors
		{
			"output",
			0,
			uapi.HandleRequestOutput,
			uapi.EventRequestBothEdges,
			2,
			unix.EINVAL,
		},
		{
			"oorange",
			0,
			uapi.HandleRequestInput,
			uapi.EventRequestBothEdges,
			6,
			unix.EINVAL,
		},
		{
			"input drain",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestOpenDrain,
			uapi.EventRequestBothEdges,
			2,
			unix.EINVAL,
		},
		{
			"input source",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestOpenSource,
			uapi.EventRequestBothEdges,
			2,
			unix.EINVAL,
		},
		{
			"as-is drain",
			0,
			uapi.HandleRequestOpenDrain,
			uapi.EventRequestBothEdges,
			2,
			unix.EINVAL,
		},
		{
			"as-is source",
			0,
			uapi.HandleRequestOpenSource,
			uapi.EventRequestBothEdges,
			2,
			unix.EINVAL,
		},
		{
			"bias disable and pull-up",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestBiasDisable |
				uapi.HandleRequestPullUp,
			uapi.EventRequestBothEdges,
			2,
			unix.EINVAL,
		},
		{
			"bias disable and pull-down",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestBiasDisable |
				uapi.HandleRequestPullDown,
			uapi.EventRequestBothEdges,
			2,
			unix.EINVAL,
		},
		{
			"pull-up and pull-down",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestPullUp |
				uapi.HandleRequestPullDown,
			uapi.EventRequestBothEdges,
			2,
			unix.EINVAL,
		},
	}
	for _, p := range patterns {
		c, err := mock.Chip(p.cnum)
		require.Nil(t, err)
		tf := func(t *testing.T) {
			f, err := os.Open(c.DevPath)
			require.Nil(t, err)
			defer f.Close()
			er := uapi.EventRequest{
				Offset:      p.offset,
				HandleFlags: p.handleFlag,
			}
			copy(er.Consumer[:], p.name)
			err = uapi.GetLineEvent(f.Fd(), &er)
			assert.Equal(t, p.err, err)
			if p.offset > uint32(c.Lines) {
				return
			}
			li, err := uapi.GetLineInfo(f.Fd(), int(p.offset))
			assert.Nil(t, err)
			if p.err != nil {
				assert.False(t, li.Flags.IsRequested())
				unix.Close(int(er.Fd))
				return
			}
			xli := uapi.LineInfo{
				Offset: p.offset,
				Flags:  uapi.LineFlagRequested | lineFromHandle(p.handleFlag),
			}
			copy(xli.Name[:], li.Name[:]) // don't care about name
			copy(xli.Consumer[:31], p.name)
			assert.Equal(t, xli, li)
			unix.Close(int(er.Fd))
		}
		t.Run(p.name, tf)
	}
}

func TestGetLineHandle(t *testing.T) {
	requireMockup(t)
	patterns := []struct {
		name       string // unique name for pattern (hf/ef/offsets/xval combo)
		cnum       int
		handleFlag uapi.HandleFlag
		offsets    []uint32
		err        error
	}{
		{
			"as-is",
			0,
			0,
			[]uint32{2},
			nil,
		},
		{
			"atv-lo",
			1,
			uapi.HandleRequestActiveLow,
			[]uint32{2},
			nil,
		},
		{
			"input",
			0,
			uapi.HandleRequestInput,
			[]uint32{2},
			nil,
		},
		{
			"input pull-up",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestPullUp,
			[]uint32{2},
			nil,
		},
		{
			"input pull-down",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestPullDown,
			[]uint32{3},
			nil,
		},
		{
			"input bias disable",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestBiasDisable,
			[]uint32{3},
			nil,
		},
		{
			"output",
			0,
			uapi.HandleRequestOutput,
			[]uint32{2},
			nil,
		},
		{
			"output drain",
			0,
			uapi.HandleRequestOutput |
				uapi.HandleRequestOpenDrain,
			[]uint32{2},
			nil,
		},
		{
			"output source",
			0,
			uapi.HandleRequestOutput |
				uapi.HandleRequestOpenSource,
			[]uint32{3},
			nil,
		},
		{
			"output pull-up",
			0,
			uapi.HandleRequestOutput |
				uapi.HandleRequestPullUp,
			[]uint32{1},
			nil,
		},
		{
			"output pull-down",
			0,
			uapi.HandleRequestOutput |
				uapi.HandleRequestPullDown,
			[]uint32{2},
			nil,
		},
		{
			"output bias disable",
			0,
			uapi.HandleRequestOutput |
				uapi.HandleRequestBiasDisable,
			[]uint32{2},
			nil,
		},
		// expected errors
		{
			"both io",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestOutput,
			[]uint32{2},
			unix.EINVAL,
		},
		{
			"overlength",
			0,
			uapi.HandleRequestInput,
			[]uint32{0, 1, 2, 3, 4},
			unix.EINVAL,
		},
		{
			"oorange",
			0,
			uapi.HandleRequestInput,
			[]uint32{6},
			unix.EINVAL,
		},
		{
			"input drain",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestOpenDrain,
			[]uint32{1},
			unix.EINVAL,
		},
		{
			"input source",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestOpenSource,
			[]uint32{2},
			unix.EINVAL,
		},
		{
			"as-is drain",
			0,
			uapi.HandleRequestOpenDrain,
			[]uint32{2},
			unix.EINVAL,
		},
		{
			"as-is source",
			0,
			uapi.HandleRequestOpenSource,
			[]uint32{1},
			unix.EINVAL,
		},
		{
			"drain source",
			0,
			uapi.HandleRequestOutput |
				uapi.HandleRequestOpenDrain |
				uapi.HandleRequestOpenSource,
			[]uint32{2},
			unix.EINVAL,
		},
		{
			"as-is pull-up",
			0,
			uapi.HandleRequestPullUp,
			[]uint32{1},
			unix.EINVAL,
		},
		{
			"as-is pull-down",
			0,
			uapi.HandleRequestPullDown,
			[]uint32{2},
			unix.EINVAL,
		},
		{
			"as-is bias disable",
			0,
			uapi.HandleRequestBiasDisable,
			[]uint32{2},
			unix.EINVAL,
		},
		{
			"bias disable and pull-up",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestBiasDisable |
				uapi.HandleRequestPullUp,
			[]uint32{2},
			unix.EINVAL,
		},
		{
			"bias disable and pull-down",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestBiasDisable |
				uapi.HandleRequestPullDown,
			[]uint32{2},
			unix.EINVAL,
		},
		{
			"pull-up and pull-down",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestPullUp |
				uapi.HandleRequestPullDown,
			[]uint32{2},
			unix.EINVAL,
		},
		{
			"all bias flags",
			0,
			uapi.HandleRequestInput |
				uapi.HandleRequestBiasDisable |
				uapi.HandleRequestPullUp |
				uapi.HandleRequestPullDown,
			[]uint32{2},
			unix.EINVAL,
		},
	}
	for _, p := range patterns {
		c, err := mock.Chip(p.cnum)
		require.Nil(t, err)
		tf := func(t *testing.T) {
			f, err := os.Open(c.DevPath)
			require.Nil(t, err)
			defer f.Close()
			hr := uapi.HandleRequest{
				Flags: p.handleFlag,
				Lines: uint32(len(p.offsets)),
			}
			copy(hr.Offsets[:], p.offsets)
			copy(hr.Consumer[:], p.name)
			err = uapi.GetLineHandle(f.Fd(), &hr)
			assert.Equal(t, p.err, err)
			if p.offsets[0] > uint32(c.Lines) {
				return
			}
			// check line info
			li, err := uapi.GetLineInfo(f.Fd(), int(p.offsets[0]))
			assert.Nil(t, err)
			if p.err != nil {
				assert.False(t, li.Flags.IsRequested())
				unix.Close(int(hr.Fd))
				return
			}
			xli := uapi.LineInfo{
				Offset: p.offsets[0],
				Flags:  uapi.LineFlagRequested | lineFromHandle(p.handleFlag),
			}
			copy(xli.Name[:], li.Name[:]) // don't care about name
			copy(xli.Consumer[:31], p.name)
			assert.Equal(t, xli, li)
			unix.Close(int(hr.Fd))
		}
		t.Run(p.name, tf)
	}
}

func TestGetLineValues(t *testing.T) {
	requireMockup(t)
	patterns := []struct {
		name       string // unique name for pattern (hf/ef/offsets/xval combo)
		cnum       int
		handleFlag uapi.HandleFlag
		evtFlag    uapi.EventFlag
		offsets    []uint32
		val        []uint8
	}{
		{
			"as-is atv-lo lo",
			1,
			uapi.HandleRequestActiveLow,
			0,
			[]uint32{2},
			[]uint8{0},
		},
		{
			"as-is atv-lo hi",
			1,
			uapi.HandleRequestActiveLow,
			0,
			[]uint32{2},
			[]uint8{1},
		},
		{
			"as-is lo",
			0,
			0,
			0,
			[]uint32{2},
			[]uint8{0},
		},
		{
			"as-is hi",
			0,
			0,
			0,
			[]uint32{1},
			[]uint8{1},
		},
		{
			"input lo",
			0,
			uapi.HandleRequestInput,
			0,
			[]uint32{2},
			[]uint8{0},
		},
		{
			"input hi",
			0,
			uapi.HandleRequestInput,
			0,
			[]uint32{1},
			[]uint8{1},
		},
		{
			"output lo",
			0,
			uapi.HandleRequestOutput,
			0,
			[]uint32{2},
			[]uint8{0},
		},
		{
			"output hi",
			0,
			uapi.HandleRequestOutput,
			0,
			[]uint32{1},
			[]uint8{1},
		},
		{
			"both lo",
			1,
			0,
			uapi.EventRequestBothEdges,
			[]uint32{2},
			[]uint8{0},
		},
		{
			"both hi",
			1,
			0,
			uapi.EventRequestBothEdges,
			[]uint32{1},
			[]uint8{1},
		},
		{
			"falling lo",
			0,
			0,
			uapi.EventRequestFallingEdge,
			[]uint32{2},
			[]uint8{0},
		},
		{
			"falling hi",
			0,
			0,
			uapi.EventRequestFallingEdge,
			[]uint32{1},
			[]uint8{1},
		},
		{
			"rising lo",
			0,
			0,
			uapi.EventRequestRisingEdge,
			[]uint32{2},
			[]uint8{0},
		},
		{
			"rising hi",
			0,
			0,
			uapi.EventRequestRisingEdge,
			[]uint32{1},
			[]uint8{1},
		},
		{
			"input 2a",
			0,
			uapi.HandleRequestInput,
			0,
			[]uint32{0, 1},
			[]uint8{1, 0},
		},
		{
			"input 2b",
			0,
			uapi.HandleRequestInput,
			0,
			[]uint32{2, 1},
			[]uint8{0, 1},
		},
		{
			"input 3a",
			0,
			uapi.HandleRequestInput,
			0,
			[]uint32{0, 1, 2},
			[]uint8{0, 1, 1},
		},
		{
			"input 3b",
			0,
			uapi.HandleRequestInput,
			0,
			[]uint32{0, 2, 1},
			[]uint8{0, 1, 0},
		},
		{
			"input 4a",
			0,
			uapi.HandleRequestInput,
			0,
			[]uint32{0, 1, 2, 3},
			[]uint8{0, 1, 1, 1},
		},
		{
			"input 4b",
			0,
			uapi.HandleRequestInput,
			0,
			[]uint32{3, 2, 1, 0},
			[]uint8{1, 1, 0, 1},
		},
		{
			"input 8a",
			1,
			uapi.HandleRequestInput,
			0,
			[]uint32{0, 1, 2, 3, 4, 5, 6, 7},
			[]uint8{0, 1, 1, 1, 1, 1, 0, 0},
		},
		{
			"input 8b",
			1,
			uapi.HandleRequestInput,
			0,
			[]uint32{3, 2, 1, 0, 4, 5, 6, 7},
			[]uint8{1, 1, 0, 1, 1, 1, 0, 1},
		},
		{
			"atv-lo 8b",
			1,
			uapi.HandleRequestInput |
				uapi.HandleRequestActiveLow,
			0,
			[]uint32{3, 2, 1, 0, 4, 6, 7},
			[]uint8{1, 1, 0, 1, 1, 1, 0, 0},
		},
	}
	for _, p := range patterns {
		c, err := mock.Chip(p.cnum)
		require.Nil(t, err)
		// set vals in mock
		require.LessOrEqual(t, len(p.offsets), len(p.val))
		for i, o := range p.offsets {
			v := int(p.val[i])
			if p.handleFlag.IsActiveLow() {
				v ^= 0x01 // assumes using 1 for high
			}
			err := c.SetValue(int(o), v)
			assert.Nil(t, err)
		}
		tf := func(t *testing.T) {
			f, err := os.Open(c.DevPath)
			require.Nil(t, err)
			defer f.Close()
			var fd int32
			xval := p.val
			if p.evtFlag == 0 {
				hr := uapi.HandleRequest{
					Flags: p.handleFlag,
					Lines: uint32(len(p.offsets)),
				}
				copy(hr.Offsets[:], p.offsets)
				err := uapi.GetLineHandle(f.Fd(), &hr)
				require.Nil(t, err)
				fd = hr.Fd
				if p.handleFlag.IsOutput() {
					// mock is ignored for outputs
					xval = make([]uint8, len(p.val))
				}
			} else {
				assert.Equal(t, 1, len(p.offsets)) // reminder that events are limited to one line
				er := uapi.EventRequest{
					Offset:      p.offsets[0],
					HandleFlags: p.handleFlag,
					EventFlags:  p.evtFlag,
				}
				err = uapi.GetLineEvent(f.Fd(), &er)
				require.Nil(t, err)
				fd = er.Fd
			}
			var hdx uapi.HandleData
			copy(hdx[:], xval)
			var hd uapi.HandleData
			err = uapi.GetLineValues(uintptr(fd), &hd)
			assert.Nil(t, err)
			assert.Equal(t, hdx, hd)
			unix.Close(int(fd))
		}
		t.Run(p.name, tf)
	}
	// badfd
	var hdx uapi.HandleData
	var hd uapi.HandleData
	err := uapi.GetLineValues(0, &hd)
	assert.NotNil(t, err)
	assert.Equal(t, hdx, hd)
}

func TestSetLineValues(t *testing.T) {
	requireMockup(t)
	patterns := []struct {
		name       string // unique name for pattern (hf/ef/offsets/xval combo)
		cnum       int
		handleFlag uapi.HandleFlag
		offsets    []uint32
		val        []uint8
		err        error
	}{
		{
			"output atv-lo lo",
			1,
			uapi.HandleRequestOutput |
				uapi.HandleRequestActiveLow,
			[]uint32{2},
			[]uint8{0},
			nil,
		},
		{
			"output atv-lo hi",
			1,
			uapi.HandleRequestOutput |
				uapi.HandleRequestActiveLow,
			[]uint32{2},
			[]uint8{1},
			nil,
		},
		{
			"as-is lo",
			1,
			0,
			[]uint32{2},
			[]uint8{0},
			nil,
		},
		{
			"as-is hi",
			1,
			0,
			[]uint32{2},
			[]uint8{1},
			nil,
		},
		{
			"output lo",
			0,
			uapi.HandleRequestOutput,
			[]uint32{2},
			[]uint8{0},
			nil,
		},
		{
			"output hi",
			0,
			uapi.HandleRequestOutput,
			[]uint32{1},
			[]uint8{1},
			nil,
		},
		{
			"output 2a",
			0,
			uapi.HandleRequestOutput,
			[]uint32{0, 1},
			[]uint8{1, 0},
			nil,
		},
		{
			"output 2b",
			0,
			uapi.HandleRequestOutput,
			[]uint32{2, 1},
			[]uint8{0, 1},
			nil,
		},
		{
			"output 3a",
			0,
			uapi.HandleRequestOutput,
			[]uint32{0, 1, 2},
			[]uint8{0, 1, 1},
			nil,
		},
		{
			"output 3b",
			0,
			uapi.HandleRequestOutput,
			[]uint32{0, 2, 1},
			[]uint8{0, 1, 0},
			nil,
		},
		{
			"output 4a",
			0,
			uapi.HandleRequestOutput,
			[]uint32{0, 1, 2, 3},
			[]uint8{0, 1, 1, 1},
			nil,
		},
		{
			"output 4b",
			0,
			uapi.HandleRequestOutput,
			[]uint32{3, 2, 1, 0},
			[]uint8{1, 1, 0, 1},
			nil,
		},
		{
			"output 8a",
			1,
			uapi.HandleRequestOutput,
			[]uint32{0, 1, 2, 3, 4, 5, 6, 7},
			[]uint8{0, 1, 1, 1, 1, 1, 0, 0},
			nil,
		},
		{
			"output 8b",
			1,
			uapi.HandleRequestOutput,
			[]uint32{3, 2, 1, 0, 4, 5, 6, 7},
			[]uint8{1, 1, 0, 1, 1, 1, 0, 1},
			nil,
		},
		{
			"atv-lo 8b",
			1,
			uapi.HandleRequestOutput |
				uapi.HandleRequestActiveLow,
			[]uint32{3, 2, 1, 0, 4, 5, 6, 7},
			[]uint8{1, 1, 0, 1, 1, 0, 0, 0},
			nil,
		},
		// expected failures....
		{
			"input lo",
			0,
			uapi.HandleRequestInput,
			[]uint32{2},
			[]uint8{0},
			unix.EPERM,
		},
		{
			"input hi",
			0,
			uapi.HandleRequestInput,
			[]uint32{1},
			[]uint8{1},
			unix.EPERM,
		},
	}
	for _, p := range patterns {
		tf := func(t *testing.T) {
			c, err := mock.Chip(p.cnum)
			require.Nil(t, err)
			require.LessOrEqual(t, len(p.offsets), len(p.val))
			f, err := os.Open(c.DevPath)
			require.Nil(t, err)
			defer f.Close()
			hr := uapi.HandleRequest{
				Flags: p.handleFlag,
				Lines: uint32(len(p.offsets)),
			}
			copy(hr.Offsets[:], p.offsets)
			err = uapi.GetLineHandle(f.Fd(), &hr)
			require.Nil(t, err)
			var hd uapi.HandleData
			copy(hd[:], p.val)
			err = uapi.SetLineValues(uintptr(hr.Fd), hd)
			assert.Equal(t, p.err, err)
			if p.err == nil {
				// check values from mock
				for i, o := range p.offsets {
					v, err := c.Value(int(o))
					assert.Nil(t, err)
					xv := int(p.val[i])
					if p.handleFlag.IsActiveLow() {
						xv ^= 0x01 // assumes using 1 for high
					}
					assert.Equal(t, xv, v)
				}
			}
			unix.Close(int(hr.Fd))
		}
		t.Run(p.name, tf)
	}
	// badfd
	var hd uapi.HandleData
	err := uapi.SetLineValues(0, hd)
	assert.NotNil(t, err)
}

func TestSetLineHandleConfig(t *testing.T) {
	requireMockup(t)
	patterns := []struct {
		name        string
		cnum        int
		offsets     []uint32
		initialFlag uapi.HandleFlag
		initialVal  []uint8
		configFlag  uapi.HandleFlag
		configVal   []uint8
		err         error
	}{
		{
			"in to out",
			1,
			[]uint32{1, 2, 3},
			uapi.HandleRequestInput,
			[]uint8{0, 1, 1},
			uapi.HandleRequestOutput,
			[]uint8{1, 0, 1},
			nil,
		},
		{
			"out to in",
			0,
			[]uint32{2},
			uapi.HandleRequestOutput,
			[]uint8{1, 0, 1},
			uapi.HandleRequestInput,
			[]uint8{1, 0, 1},
			nil,
		},
		{
			"as-is atv-hi to as-is atv-lo",
			0,
			[]uint32{1, 2, 3},
			0,
			[]uint8{1, 0, 1},
			uapi.HandleRequestActiveLow,
			[]uint8{1, 0, 1},
			nil,
		},
		{
			"as-is atv-lo to as-is atv-hi",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestActiveLow,
			[]uint8{1, 0, 1},
			0,
			[]uint8{1, 0, 1},
			nil,
		},
		{
			"input atv-lo to input atv-hi",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestInput |
				uapi.HandleRequestActiveLow,
			[]uint8{1, 0, 1},
			uapi.HandleRequestInput,
			[]uint8{1, 0, 1},
			nil,
		},
		{
			"input atv-hi to input atv-lo",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestInput,
			[]uint8{1, 0, 1},
			uapi.HandleRequestInput |
				uapi.HandleRequestActiveLow,
			[]uint8{1, 0, 1},
			nil,
		},
		{
			"output atv-lo to output atv-hi",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestOutput |
				uapi.HandleRequestActiveLow,
			[]uint8{1, 0, 1},
			uapi.HandleRequestOutput,
			[]uint8{0, 1, 1},
			nil,
		},
		{
			"output atv-hi to output atv-lo",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestOutput,
			[]uint8{1, 0, 1},
			uapi.HandleRequestOutput |
				uapi.HandleRequestActiveLow,
			[]uint8{0, 1, 1},
			nil,
		},
		{
			"input atv-lo to as-is atv-hi",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestInput |
				uapi.HandleRequestActiveLow,
			[]uint8{1, 0, 1},
			0,
			[]uint8{1, 0, 1},
			nil,
		},
		{
			"input atv-hi to as-is atv-lo",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestInput,
			[]uint8{1, 0, 1},
			uapi.HandleRequestActiveLow,
			[]uint8{1, 0, 1},
			nil,
		},
		{
			"input pull-up to input pull-down",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestInput |
				uapi.HandleRequestPullUp,
			nil,
			uapi.HandleRequestInput |
				uapi.HandleRequestPullDown,
			[]uint8{0, 0, 0},
			nil,
		},
		{
			"input pull-down to input pull-up",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestInput |
				uapi.HandleRequestPullDown,
			nil,
			uapi.HandleRequestInput |
				uapi.HandleRequestPullUp,
			[]uint8{1, 1, 1},
			nil,
		},
		{
			"output atv-lo to as-is atv-hi",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestOutput |
				uapi.HandleRequestActiveLow,
			[]uint8{1, 0, 1},
			0,
			[]uint8{0, 1, 0}, // must be opposite of initial as mock is not updated
			nil,
		},
		{
			"output atv-hi to as-is atv-lo",
			0,
			[]uint32{1, 2, 3},
			uapi.HandleRequestOutput,
			[]uint8{1, 0, 1},
			uapi.HandleRequestActiveLow,
			[]uint8{0, 1, 0}, // must be opposite of initial as mock is not updated
			nil,
		},
		// expected errors
		{
			"input drain",
			0,
			[]uint32{2},
			uapi.HandleRequestInput,
			nil,
			uapi.HandleRequestInput |
				uapi.HandleRequestOpenDrain,
			nil,
			unix.EINVAL,
		},
		{
			"input source",
			0,
			[]uint32{2},
			uapi.HandleRequestInput,
			nil,
			uapi.HandleRequestInput |
				uapi.HandleRequestOpenSource,
			nil,
			unix.EINVAL,
		},
		{
			"as-is drain",
			0,
			[]uint32{2},
			0,
			nil,
			uapi.HandleRequestOpenDrain,
			nil,
			unix.EINVAL,
		},
		{
			"as-is source",
			0,
			[]uint32{2},
			0,
			nil,
			uapi.HandleRequestOpenSource,
			nil,
			unix.EINVAL,
		},
		{
			"drain source",
			0,
			[]uint32{2},
			uapi.HandleRequestOutput,
			nil,
			uapi.HandleRequestOutput |
				uapi.HandleRequestOpenDrain |
				uapi.HandleRequestOpenSource,
			nil,
			unix.EINVAL,
		},
		{
			"pull-up and pull-down",
			0,
			[]uint32{2},
			uapi.HandleRequestOutput,
			nil,
			uapi.HandleRequestOutput |
				uapi.HandleRequestPullUp |
				uapi.HandleRequestPullDown,
			nil,
			unix.EINVAL,
		},
		{
			"bias disable and pull-up",
			0,
			[]uint32{2},
			uapi.HandleRequestOutput,
			nil,
			uapi.HandleRequestOutput |
				uapi.HandleRequestBiasDisable |
				uapi.HandleRequestPullUp,
			nil,
			unix.EINVAL,
		},
		{
			"bias disable and pull-down",
			0,
			[]uint32{2},
			uapi.HandleRequestOutput,
			nil,
			uapi.HandleRequestOutput |
				uapi.HandleRequestBiasDisable |
				uapi.HandleRequestPullDown,
			nil,
			unix.EINVAL,
		},
		{
			"all bias flags",
			0,
			[]uint32{2},
			uapi.HandleRequestOutput,
			nil,
			uapi.HandleRequestOutput |
				uapi.HandleRequestBiasDisable |
				uapi.HandleRequestPullDown |
				uapi.HandleRequestPullUp,
			nil,
			unix.EINVAL,
		},
	}
	for _, p := range patterns {
		tf := func(t *testing.T) {
			c, err := mock.Chip(p.cnum)
			require.Nil(t, err)
			// setup mockup for inputs
			if p.initialVal != nil {
				for i, o := range p.offsets {
					v := int(p.initialVal[i])
					// read is after config, so use config active state
					if p.configFlag.IsActiveLow() {
						v ^= 0x01 // assumes using 1 for high
					}
					err := c.SetValue(int(o), v)
					assert.Nil(t, err)
				}
			}
			f, err := os.Open(c.DevPath)
			require.Nil(t, err)
			defer f.Close()
			hr := uapi.HandleRequest{
				Flags: p.initialFlag,
				Lines: uint32(len(p.offsets)),
			}
			copy(hr.Offsets[:], p.offsets)
			copy(hr.DefaultValues[:], p.initialVal)
			copy(hr.Consumer[:], p.name)
			err = uapi.GetLineHandle(f.Fd(), &hr)
			require.Nil(t, err)
			// apply config change
			hc := uapi.HandleConfig{Flags: p.configFlag}
			copy(hc.DefaultValues[:], p.configVal)
			err = uapi.SetLineConfig(uintptr(hr.Fd), &hc)
			assert.Equal(t, p.err, err)

			if p.err == nil {
				// check line info
				li, err := uapi.GetLineInfo(f.Fd(), int(p.offsets[0]))
				assert.Nil(t, err)
				if p.err != nil {
					assert.False(t, li.Flags.IsRequested())
					return
				}
				xli := uapi.LineInfo{
					Offset: p.offsets[0],
					Flags: uapi.LineFlagRequested |
						lineFromConfig(p.initialFlag, p.configFlag),
				}
				copy(xli.Name[:], li.Name[:]) // don't care about name
				copy(xli.Consumer[:31], p.name)
				assert.Equal(t, xli, li)
				if len(p.configVal) != 0 {
					// check values from mock
					require.LessOrEqual(t, len(p.offsets), len(p.configVal))
					for i, o := range p.offsets {
						v, err := c.Value(int(o))
						assert.Nil(t, err)
						xv := int(p.configVal[i])
						if p.configFlag.IsActiveLow() {
							xv ^= 0x01 // assumes using 1 for high
						}
						assert.Equal(t, xv, v, i)
					}
				}
			}
			unix.Close(int(hr.Fd))
		}
		t.Run(p.name, tf)
	}
}

func TestSetLineEventConfig(t *testing.T) {
	requireKernel(t, setConfigKernel)
	requireMockup(t)
	patterns := []struct {
		name        string
		cnum        int
		offset      uint32
		initialFlag uapi.HandleFlag
		configFlag  uapi.HandleFlag
		err         error
	}{
		// expected errors
		{
			"low to high", 0, 1,
			uapi.HandleRequestInput |
				uapi.HandleRequestActiveLow,
			0,
			unix.EINVAL,
		},
		{
			"high to low", 0, 2,
			uapi.HandleRequestInput,
			uapi.HandleRequestInput |
				uapi.HandleRequestActiveLow,
			unix.EINVAL,
		},
		{
			"in to out", 1, 2,
			uapi.HandleRequestInput,
			uapi.HandleRequestOutput,
			unix.EINVAL,
		},
		{
			"drain", 0, 3,
			uapi.HandleRequestInput,
			uapi.HandleRequestInput |
				uapi.HandleRequestOpenDrain,
			unix.EINVAL,
		},
		{
			"source", 0, 3,
			uapi.HandleRequestInput,
			uapi.HandleRequestInput |
				uapi.HandleRequestOpenSource,
			unix.EINVAL,
		},
	}
	for _, p := range patterns {
		tf := func(t *testing.T) {
			c, err := mock.Chip(p.cnum)
			require.Nil(t, err)
			f, err := os.Open(c.DevPath)
			require.Nil(t, err)
			defer f.Close()
			er := uapi.EventRequest{
				HandleFlags: p.initialFlag,
				EventFlags:  uapi.EventRequestBothEdges,
				Offset:      p.offset,
			}
			copy(er.Consumer[:], p.name)
			err = uapi.GetLineEvent(f.Fd(), &er)
			require.Nil(t, err)
			// apply config change
			hc := uapi.HandleConfig{Flags: p.configFlag}
			err = uapi.SetLineConfig(uintptr(er.Fd), &hc)
			assert.Equal(t, p.err, err)

			if p.err == nil {
				// check line info
				li, err := uapi.GetLineInfo(f.Fd(), int(p.offset))
				assert.Nil(t, err)
				if p.err != nil {
					assert.False(t, li.Flags.IsRequested())
					return
				}
				xli := uapi.LineInfo{
					Offset: p.offset,
					Flags: uapi.LineFlagRequested |
						lineFromConfig(p.initialFlag, p.configFlag),
				}
				copy(xli.Name[:], li.Name[:]) // don't care about name
				copy(xli.Consumer[:31], p.name)
				assert.Equal(t, xli, li)
			}
			unix.Close(int(er.Fd))
		}
		t.Run(p.name, tf)
	}
}

func TestWatchLineInfo(t *testing.T) {
	// also covers ReadLineInfoChanged

	requireKernel(t, infoWatchKernel)
	requireMockup(t)
	c, err := mock.Chip(0)
	require.Nil(t, err)

	f, err := os.Open(c.DevPath)
	require.Nil(t, err)
	defer f.Close()

	// unwatched
	hr := uapi.HandleRequest{Lines: 1, Flags: uapi.HandleRequestInput}
	hr.Offsets[0] = 3
	copy(hr.Consumer[:], "testwatch")
	err = uapi.GetLineHandle(f.Fd(), &hr)
	assert.Nil(t, err)
	chg, err := readLineInfoChangedTimeout(f.Fd(), spuriousEventWaitTimeout)
	assert.Nil(t, err)
	assert.Nil(t, chg, "spurious change")
	unix.Close(int(hr.Fd))

	// out of range
	li := uapi.LineInfo{Offset: uint32(c.Lines + 1)}
	err = uapi.WatchLineInfo(f.Fd(), &li)
	require.Equal(t, syscall.Errno(0x16), err)

	// set watch
	li = uapi.LineInfo{Offset: 3}
	lname := c.Label + "-3"
	err = uapi.WatchLineInfo(f.Fd(), &li)
	require.Nil(t, err)
	xli := uapi.LineInfo{Offset: 3}
	copy(xli.Name[:], lname)
	assert.Equal(t, xli, li)

	// repeated watch
	err = uapi.WatchLineInfo(f.Fd(), &li)
	assert.Equal(t, unix.EBUSY, err)

	chg, err = readLineInfoChangedTimeout(f.Fd(), spuriousEventWaitTimeout)
	assert.Nil(t, err)
	assert.Nil(t, chg, "spurious change")

	// request line
	hr = uapi.HandleRequest{Lines: 1, Flags: uapi.HandleRequestInput}
	hr.Offsets[0] = 3
	copy(hr.Consumer[:], "testwatch")
	err = uapi.GetLineHandle(f.Fd(), &hr)
	assert.Nil(t, err)
	chg, err = readLineInfoChangedTimeout(f.Fd(), eventWaitTimeout)
	assert.Nil(t, err)
	require.NotNil(t, chg)
	assert.Equal(t, uapi.LineChangedRequested, chg.Type)
	xli.Flags |= uapi.LineFlagRequested
	copy(xli.Consumer[:], "testwatch")
	assert.Equal(t, xli, chg.Info)

	chg, err = readLineInfoChangedTimeout(f.Fd(), spuriousEventWaitTimeout)
	assert.Nil(t, err)
	assert.Nil(t, chg, "spurious change")

	// reconfig line
	hc := uapi.HandleConfig{Flags: uapi.HandleRequestActiveLow}
	copy(hr.Consumer[:], "testwatch")
	err = uapi.SetLineConfig(uintptr(hr.Fd), &hc)
	assert.Nil(t, err)
	chg, err = readLineInfoChangedTimeout(f.Fd(), eventWaitTimeout)
	assert.Nil(t, err)
	require.NotNil(t, chg)
	assert.Equal(t, uapi.LineChangedConfig, chg.Type)
	xli.Flags |= uapi.LineFlagActiveLow
	assert.Equal(t, xli, chg.Info)

	chg, err = readLineInfoChangedTimeout(f.Fd(), spuriousEventWaitTimeout)
	assert.Nil(t, err)
	assert.Nil(t, chg, "spurious change")

	// release line
	unix.Close(int(hr.Fd))
	chg, err = readLineInfoChangedTimeout(f.Fd(), eventWaitTimeout)
	assert.Nil(t, err)
	require.NotNil(t, chg)
	assert.Equal(t, uapi.LineChangedReleased, chg.Type)
	xli = uapi.LineInfo{Offset: 3}
	copy(xli.Name[:], lname)
	assert.Equal(t, xli, chg.Info)

	chg, err = readLineInfoChangedTimeout(f.Fd(), spuriousEventWaitTimeout)
	assert.Nil(t, err)
	assert.Nil(t, chg, "spurious change")
}

func TestUnwatchLineInfo(t *testing.T) {
	requireKernel(t, infoWatchKernel)
	requireMockup(t)
	c, err := mock.Chip(0)
	require.Nil(t, err)

	f, err := os.Open(c.DevPath)
	require.Nil(t, err)
	defer f.Close()

	li := uapi.LineInfo{Offset: uint32(c.Lines + 1)}
	err = uapi.UnwatchLineInfo(f.Fd(), li.Offset)
	require.Equal(t, syscall.Errno(0x16), err)

	li = uapi.LineInfo{Offset: 3}
	lname := c.Label + "-3"
	err = uapi.WatchLineInfo(f.Fd(), &li)
	require.Nil(t, err)
	xli := uapi.LineInfo{Offset: 3}
	copy(xli.Name[:], lname)
	assert.Equal(t, xli, li)

	chg, err := readLineInfoChangedTimeout(f.Fd(), spuriousEventWaitTimeout)
	assert.Nil(t, err)
	assert.Nil(t, chg, "spurious change")

	err = uapi.UnwatchLineInfo(f.Fd(), li.Offset)
	assert.Nil(t, err)

	// request line
	hr := uapi.HandleRequest{Lines: 1, Flags: uapi.HandleRequestInput}
	hr.Offsets[0] = 3
	err = uapi.GetLineHandle(f.Fd(), &hr)
	assert.Nil(t, err)
	unix.Close(int(hr.Fd))
	chg, err = readLineInfoChangedTimeout(f.Fd(), spuriousEventWaitTimeout)
	assert.Nil(t, err)
	assert.Nil(t, chg, "spurious change")

	// repeated unwatch
	err = uapi.UnwatchLineInfo(f.Fd(), 3)
	require.Equal(t, unix.EBUSY, err)

	// repeated watch
	err = uapi.WatchLineInfo(f.Fd(), &li)
	require.Nil(t, err)
}

func TestReadEvent(t *testing.T) {
	requireMockup(t)
	c, err := mock.Chip(0)
	require.Nil(t, err)
	f, err := os.Open(c.DevPath)
	require.Nil(t, err)
	defer f.Close()
	err = c.SetValue(1, 0)
	require.Nil(t, err)
	er := uapi.EventRequest{
		Offset: 1,
		HandleFlags: uapi.HandleRequestInput |
			uapi.HandleRequestActiveLow,
		EventFlags: uapi.EventRequestBothEdges,
	}
	err = uapi.GetLineEvent(f.Fd(), &er)
	require.Nil(t, err)

	evt, err := readEventTimeout(er.Fd, spuriousEventWaitTimeout)
	assert.Nil(t, err)
	assert.Nil(t, evt, "spurious event")

	c.SetValue(1, 1)
	evt, err = readEventTimeout(er.Fd, eventWaitTimeout)
	require.Nil(t, err)
	require.NotNil(t, evt)
	assert.Equal(t, uint32(2), evt.ID) // returns falling edge

	c.SetValue(1, 0)
	evt, err = readEventTimeout(er.Fd, eventWaitTimeout)
	assert.Nil(t, err)
	require.NotNil(t, evt)
	assert.Equal(t, uint32(1), evt.ID) // returns rising edge

	unix.Close(int(er.Fd))
}

func readEventTimeout(fd int32, t time.Duration) (*uapi.EventData, error) {
	pollfd := unix.PollFd{Fd: int32(fd), Events: unix.POLLIN}
	n, err := unix.Poll([]unix.PollFd{pollfd}, int(t.Milliseconds()))
	if err != nil || n != 1 {
		return nil, err
	}
	evt, err := uapi.ReadEvent(uintptr(fd))
	if err != nil {
		return nil, err
	}
	return &evt, nil
}

func readLineInfoChangedTimeout(fd uintptr,
	t time.Duration) (*uapi.LineInfoChanged, error) {

	pollfd := unix.PollFd{Fd: int32(fd), Events: unix.POLLIN}
	n, err := unix.Poll([]unix.PollFd{pollfd}, int(t.Milliseconds()))
	if err != nil || n != 1 {
		return nil, err
	}
	infoChanged, err := uapi.ReadLineInfoChanged(fd)
	if err != nil {
		return nil, err
	}
	return &infoChanged, nil
}

func TestBytesToString(t *testing.T) {
	name := "a test string"
	a := [20]byte{}
	copy(a[:], name)

	// empty
	v := uapi.BytesToString(a[:0])
	assert.Equal(t, 0, len(v))

	// normal
	v = uapi.BytesToString(a[:])
	assert.Equal(t, name, v)

	// unterminated
	v = uapi.BytesToString(a[:len(name)])
	assert.Equal(t, name, v)
}
func TestLineFlags(t *testing.T) {
	assert.False(t, uapi.LineFlag(0).IsRequested())
	assert.False(t, uapi.LineFlag(0).IsOut())
	assert.False(t, uapi.LineFlag(0).IsActiveLow())
	assert.False(t, uapi.LineFlag(0).IsOpenDrain())
	assert.False(t, uapi.LineFlag(0).IsOpenSource())
	assert.True(t, uapi.LineFlagRequested.IsRequested())
	assert.True(t, uapi.LineFlagIsOut.IsOut())
	assert.True(t, uapi.LineFlagActiveLow.IsActiveLow())
	assert.True(t, uapi.LineFlagOpenDrain.IsOpenDrain())
	assert.False(t, uapi.LineFlagOpenDrain.IsOpenSource())
	assert.True(t, uapi.LineFlagOpenSource.IsOpenSource())
	assert.False(t, uapi.LineFlagOpenSource.IsOpenDrain())
	assert.True(t, uapi.LineFlagPullUp.IsPullUp())
	assert.False(t, uapi.LineFlagPullUp.IsPullDown())
	assert.False(t, uapi.LineFlagPullUp.IsBiasDisable())
	assert.True(t, uapi.LineFlagPullDown.IsPullDown())
	assert.False(t, uapi.LineFlagPullDown.IsBiasDisable())
	assert.False(t, uapi.LineFlagPullDown.IsPullUp())
	assert.True(t, uapi.LineFlagBiasDisable.IsBiasDisable())
	assert.False(t, uapi.LineFlagBiasDisable.IsPullUp())
	assert.False(t, uapi.LineFlagBiasDisable.IsPullDown())
}

func TestHandleFlags(t *testing.T) {
	assert.False(t, uapi.HandleFlag(0).IsInput())
	assert.False(t, uapi.HandleFlag(0).IsOutput())
	assert.False(t, uapi.HandleFlag(0).IsActiveLow())
	assert.False(t, uapi.HandleFlag(0).IsOpenDrain())
	assert.False(t, uapi.HandleFlag(0).IsOpenSource())
	assert.True(t, uapi.HandleRequestInput.IsInput())
	assert.True(t, uapi.HandleRequestOutput.IsOutput())
	assert.True(t, uapi.HandleRequestActiveLow.IsActiveLow())
	assert.True(t, uapi.HandleRequestOpenDrain.IsOpenDrain())
	assert.False(t, uapi.HandleRequestOpenDrain.IsOpenSource())
	assert.True(t, uapi.HandleRequestOpenSource.IsOpenSource())
	assert.False(t, uapi.HandleRequestOpenSource.IsOpenDrain())
	assert.True(t, uapi.HandleRequestPullUp.IsPullUp())
	assert.False(t, uapi.HandleRequestPullUp.IsPullDown())
	assert.False(t, uapi.HandleRequestPullUp.IsBiasDisable())
	assert.True(t, uapi.HandleRequestPullDown.IsPullDown())
	assert.False(t, uapi.HandleRequestPullDown.IsBiasDisable())
	assert.False(t, uapi.HandleRequestPullDown.IsPullUp())
	assert.True(t, uapi.HandleRequestBiasDisable.IsBiasDisable())
	assert.False(t, uapi.HandleRequestBiasDisable.IsPullUp())
	assert.False(t, uapi.HandleRequestBiasDisable.IsPullDown())
}

func TestEventFlags(t *testing.T) {
	assert.False(t, uapi.EventRequestFallingEdge.IsBothEdges())
	assert.True(t, uapi.EventRequestFallingEdge.IsFallingEdge())
	assert.False(t, uapi.EventRequestFallingEdge.IsRisingEdge())
	assert.False(t, uapi.EventRequestRisingEdge.IsBothEdges())
	assert.False(t, uapi.EventRequestRisingEdge.IsFallingEdge())
	assert.True(t, uapi.EventRequestRisingEdge.IsRisingEdge())
	assert.True(t, uapi.EventRequestBothEdges.IsBothEdges())
	assert.True(t, uapi.EventRequestBothEdges.IsFallingEdge())
	assert.True(t, uapi.EventRequestBothEdges.IsRisingEdge())
}

func lineFromConfig(of, cf uapi.HandleFlag) uapi.LineFlag {
	lf := lineFromHandle(cf)
	if !(cf.IsInput() || cf.IsOutput()) {
		if of.IsOutput() {
			lf |= uapi.LineFlagIsOut
		}
	}
	return lf
}

func lineFromHandle(hf uapi.HandleFlag) uapi.LineFlag {
	var lf uapi.LineFlag
	if hf.IsOutput() {
		lf |= uapi.LineFlagIsOut
	}
	if hf.IsActiveLow() {
		lf |= uapi.LineFlagActiveLow
	}
	if hf.IsOpenDrain() {
		lf |= uapi.LineFlagOpenDrain
	}
	if hf.IsOpenSource() {
		lf |= uapi.LineFlagOpenSource
	}
	if hf.IsPullUp() {
		lf |= uapi.LineFlagPullUp
	}
	if hf.IsPullDown() {
		lf |= uapi.LineFlagPullDown
	}
	if hf.IsBiasDisable() {
		lf |= uapi.LineFlagBiasDisable
	}
	return lf
}

func requireKernel(t *testing.T, min mockup.Semver) {
	if err := mockup.CheckKernelVersion(min); err != nil {
		t.Skip(err)
	}
}
